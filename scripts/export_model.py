#!/usr/bin/env python
"""
Export a POS-tagging, sentence-transformer, or dependency-parsing model to ONNX.

Supported models:
  mobilebert  mrm8488/mobilebert-finetuned-pos  (fast, ~100 MB)
  bert-base   QCRI/bert-base-multilingual-cased-pos-english  (~400 MB)
  minilm      sentence-transformers/all-MiniLM-L6-v2   22 MB, 384-dim, 6-layer
  mpnet       sentence-transformers/all-mpnet-base-v2  110 MB, 768-dim, 12-layer

--model values other than mobilebert/bert-base/minilm/mpnet are treated as a
diaparser (github.com/Unipisa/diaparser) catalog name and export a biaffine
UD dependency parser (default en_ewt.electra-base) as a 3-input/2-output
ONNX graph: input_ids, attention_mask, pool_matrix -> arc_logits, rel_logits.
See _export_dependency() below for why this needs a custom wrapper rather
than exporting the diaparser checkpoint directly.

Usage:
    python export_model.py [output_dir] [--model mobilebert|bert-base|minilm|mpnet|en_ewt.electra-base]
"""
import sys
import os
import io
import argparse
import json

# Force UTF-8 stdout/stderr so torch's emoji output doesn't crash on Windows cp1252.
# sys.stdout/stderr are typed as TextIO (no .reconfigure) but are actually
# TextIOWrapper at runtime whenever they're a real console/file stream.
if isinstance(sys.stdout, io.TextIOWrapper) and sys.stdout.encoding != 'utf-8':
    sys.stdout.reconfigure(encoding='utf-8', errors='replace')
if isinstance(sys.stderr, io.TextIOWrapper) and sys.stderr.encoding != 'utf-8':
    sys.stderr.reconfigure(encoding='utf-8', errors='replace')

try:
    import torch
    from transformers import AutoModel, AutoTokenizer, AutoModelForTokenClassification
    import requests
except ImportError as e:
    print(f"Missing dependency: {e}")
    print("pip install transformers torch onnx onnxscript requests")
    sys.exit(1)

import inspect


class _DropsTokenTypeIds(torch.nn.Module):
    """Wraps a model whose forward() has no token_type_ids param (e.g. MPNet)
    so it still accepts the (input_ids, attention_mask, token_type_ids) ONNX
    input contract every other model in this script uses; the third arg is
    accepted but discarded."""
    def __init__(self, inner):
        super().__init__()
        self.inner = inner

    def forward(self, input_ids, attention_mask, token_type_ids):
        return self.inner(input_ids=input_ids, attention_mask=attention_mask)

POS_MODELS = {
    "mobilebert": "mrm8488/mobilebert-finetuned-pos",
    "bert-base":  "QCRI/bert-base-multilingual-cased-pos-english",
}
SEMANTIC_MODELS = {
    "minilm": "sentence-transformers/all-MiniLM-L6-v2",
    "mpnet": "sentence-transformers/all-mpnet-base-v2",
}
MODELS = {**POS_MODELS, **SEMANTIC_MODELS}

# diaparser (github.com/Unipisa/diaparser) catalog name used when --model is
# omitted or isn't a known POS/semantic name.
DEFAULT_DEPENDENCY_MODEL = "en_ewt.electra-base"

parser = argparse.ArgumentParser()
parser.add_argument("out_dir", nargs="?", default=os.path.dirname(os.path.abspath(__file__)))
parser.add_argument("--model", default=None,
                     help=f"One of {list(MODELS.keys())} (pos/semantic), "
                          f"or a diaparser catalog name for dependency parsing "
                          f"(default: {DEFAULT_DEPENDENCY_MODEL}).")
args = parser.parse_args()

out_dir = args.out_dir
os.makedirs(out_dir, exist_ok=True)

# Mode is inferred from --model: known POS/semantic names pick those modes;
# anything else (including no --model at all) falls through to pos's
# default bert-base, except when the name isn't a known model at all, which
# means it's a diaparser catalog name for dependency mode.
if args.model is None:
    mode = "pos"
elif args.model in POS_MODELS:
    mode = "pos"
elif args.model in SEMANTIC_MODELS:
    mode = "semantic"
else:
    mode = "dependency"


def _export_dependency(model_name, out_dir):
    """
    Export a diaparser biaffine dependency parser to ONNX.

    diaparser's own BiaffineDependencyModel can't be torch.onnx.export'd
    directly: its BertEmbedding submodule pools BERT subword outputs back to
    word-level embeddings using data-dependent ops (masked_scatter_, boolean
    masks split by `lens.tolist()`) that don't trace to a fixed ONNX graph —
    the traced graph would silently bake in one specific sentence's subword
    layout. Its BiLSTM is also a hand-rolled per-timestep LSTMCell loop over
    a PackedSequence, tied to variable-length batching machinery.

    Both problems disappear once we commit to exporting per-sentence
    (batch_size=1, no padding):
      - Subword->word pooling becomes a static matmul against a pooling
        matrix (1 row per word, mean over that word's subword columns) that
        Go builds from its own WordPiece tokenization and passes in as a
        third input (pool_matrix) — no scatter/mask ops in the graph at all.
      - With no padding, the custom LSTM's cell-based recurrence is
        mathematically identical to a stock bidirectional multi-layer
        nn.LSTM (same LSTMCell gate math per layer/direction) — so we copy
        its per-cell weights into a real nn.LSTM, which *does* export to
        ONNX cleanly.
      - We drop the optional attention-mixing bonus term (use_attentions):
        reproducing its exact subword-attention squeeze logic isn't worth
        the fragility for a bonus term added on top of the main biaffine
        arc/rel scores. (Verified against the original architecture on a
        sample sentence — see TODO.md — result was a fully correct parse.)
    """
    import torch.nn as nn
    from diaparser.parsers import Parser

    print(f"Loading diaparser model {model_name}...")
    p = Parser.load(model_name)
    m = p.model
    m.eval()

    class DepWrapper(nn.Module):
        def __init__(self, m):
            super().__init__()
            self.bert = m.feat_embed.bert
            self.scalar_mix = m.feat_embed.scalar_mix
            self.n_layers = m.feat_embed.n_layers
            old = m.lstm
            self.lstm = nn.LSTM(
                input_size=old.f_cells[0].weight_ih.shape[1],
                hidden_size=old.hidden_size,
                num_layers=old.num_layers,
                batch_first=True,
                bidirectional=True,
            )
            sd = self.lstm.state_dict()
            for i in range(old.num_layers):
                sd[f"weight_ih_l{i}"] = old.f_cells[i].weight_ih.detach().clone()
                sd[f"weight_hh_l{i}"] = old.f_cells[i].weight_hh.detach().clone()
                sd[f"bias_ih_l{i}"] = old.f_cells[i].bias_ih.detach().clone()
                sd[f"bias_hh_l{i}"] = old.f_cells[i].bias_hh.detach().clone()
                sd[f"weight_ih_l{i}_reverse"] = old.b_cells[i].weight_ih.detach().clone()
                sd[f"weight_hh_l{i}_reverse"] = old.b_cells[i].weight_hh.detach().clone()
                sd[f"bias_ih_l{i}_reverse"] = old.b_cells[i].bias_ih.detach().clone()
                sd[f"bias_hh_l{i}_reverse"] = old.b_cells[i].bias_hh.detach().clone()
            self.lstm.load_state_dict(sd)
            self.lstm.eval()
            self.mlp_arc_d = m.mlp_arc_d
            self.mlp_arc_h = m.mlp_arc_h
            self.mlp_rel_d = m.mlp_rel_d
            self.mlp_rel_h = m.mlp_rel_h
            # Re-declared (not reused via m.arc_attn/m.rel_attn's own forward()):
            # diaparser's Biaffine.forward() unconditionally does `s.squeeze(1)`,
            # a harmless no-op in eager mode when n_out != 1 (rel_attn's n_out is
            # 50), but torch.onnx bakes that squeeze in as a hard "dim must be 1"
            # assertion regardless, which then fails ONNX Runtime's shape
            # inference for rel_attn. Only squeeze when n_out is actually 1.
            self.arc_weight = m.arc_attn.weight
            self.arc_bias_x, self.arc_bias_y = m.arc_attn.bias_x, m.arc_attn.bias_y
            self.rel_weight = m.rel_attn.weight
            self.rel_bias_x, self.rel_bias_y = m.rel_attn.bias_x, m.rel_attn.bias_y

        @staticmethod
        def _biaffine(x, y, weight, bias_x, bias_y):
            if bias_x:
                x = torch.cat((x, torch.ones_like(x[..., :1])), -1)
            if bias_y:
                y = torch.cat((y, torch.ones_like(y[..., :1])), -1)
            s = torch.einsum("bxi,oij,byj->boxy", x, weight, y)
            if weight.shape[0] == 1:
                s = s.squeeze(1)
            return s

        def forward(self, input_ids, attention_mask, pool_matrix):
            out = self.bert(input_ids, attention_mask=attention_mask, output_hidden_states=True)
            hs = torch.stack(out.hidden_states[-self.n_layers:], 0)
            mixed = self.scalar_mix(hs)                    # [1, n_sub, hidden]
            word_embed = torch.matmul(pool_matrix, mixed)  # [1, seq, hidden]
            x, _ = self.lstm(word_embed)                   # [1, seq, 2*hidden]
            arc_d = self.mlp_arc_d(x)
            arc_h = self.mlp_arc_h(x)
            rel_d = self.mlp_rel_d(x)
            rel_h = self.mlp_rel_h(x)
            s_arc = self._biaffine(arc_d, arc_h, self.arc_weight, self.arc_bias_x, self.arc_bias_y)  # [1, seq, seq]
            s_rel = self._biaffine(rel_d, rel_h, self.rel_weight, self.rel_bias_x, self.rel_bias_y)  # [1, n_rels, seq, seq]
            s_rel = s_rel.permute(0, 2, 3, 1)                                                         # [1, seq, seq, n_rels]
            # Self-loop masking (a word can't be its own head) is done on the
            # Go side instead of via torch.eye here: torch.eye traces to an
            # ONNX EyeLike node that this project's onnxruntime build doesn't
            # have an implementation for.
            return s_arc, s_rel

    wrapper = DepWrapper(m)
    wrapper.eval()

    # Dummy inputs for tracing: 3 words (root + "The" + "fox", "fox" split into
    # 2 subword pieces) so the seq (3) and n_sub (4) axes differ in the trace,
    # which torch.export needs to correctly infer them as independent dims.
    dummy_ids = torch.tensor([[101, 1996, 4419, 2050]], dtype=torch.long)
    dummy_mask = torch.ones_like(dummy_ids)
    dummy_pool = torch.tensor([[
        [1.0, 0.0, 0.0, 0.0],
        [0.0, 1.0, 0.0, 0.0],
        [0.0, 0.0, 0.5, 0.5],
    ]], dtype=torch.float32)

    slug = model_name.replace("-", "_").replace("/", "_").replace(".", "_")
    onnx_path = os.path.join(out_dir, f"{slug}_dependency.onnx")
    print(f"Exporting ONNX to {onnx_path}...")
    # The new dynamo-based exporter (torch.onnx.export's default) silently
    # specialized the "seq" axis to the dummy trace's static value here (it
    # only appears once, on pool_matrix, unlike "n_sub" which several inputs
    # share) even with an explicit torch.export.Dim. The legacy TorchScript
    # exporter's plain dynamic_axes has no such guard-proving step and
    # handles this — and the LSTM ops — correctly.
    torch.onnx.export(
        wrapper,
        (dummy_ids, dummy_mask, dummy_pool),
        onnx_path,
        input_names=["input_ids", "attention_mask", "pool_matrix"],
        output_names=["arc_logits", "rel_logits"],
        dynamic_axes={
            "input_ids": {0: "batch", 1: "n_sub"},
            "attention_mask": {0: "batch", 1: "n_sub"},
            "pool_matrix": {0: "batch", 1: "seq", 2: "n_sub"},
            "arc_logits": {0: "batch", 1: "seq", 2: "seq"},
            "rel_logits": {0: "batch", 1: "seq", 2: "seq"},
        },
        opset_version=14,
        dynamo=False,
    )
    print("ONNX export done.")

    bert_model_id = m.feat_embed.model
    vocab_path = os.path.join(out_dir, f"{slug}_dependency_vocab.txt")
    vocab_url = f"https://huggingface.co/{bert_model_id}/resolve/main/vocab.txt"
    print(f"Downloading vocab.txt from {bert_model_id}...")
    r = requests.get(vocab_url)
    r.raise_for_status()
    with open(vocab_path, "wb") as f:
        f.write(r.content)
    print(f"vocab saved to {vocab_path}")

    rel_vocab = p.transform.DEPREL.vocab
    rels = [rel_vocab.itos[i] for i in range(len(rel_vocab))]
    rels_path = os.path.join(out_dir, f"{slug}_dependency_rels.json")
    with open(rels_path, "w") as f:
        json.dump(rels, f)
    print(f"rel labels saved to {rels_path} (index 0, {rels[0]!r}, is an unused placeholder)")

    print(f"\nDone. Files: {onnx_path}, {vocab_path}, {rels_path}")
    print(f"Set naturalSyntaxLs.model to {model_name!r} (with naturalSyntaxLs.mode = \"dependency\").")


if mode == "dependency":
    _export_dependency(args.model or DEFAULT_DEPENDENCY_MODEL, out_dir)
    sys.exit(0)

if args.model is None:
    args.model = "bert-base"
model_id = MODELS.get(args.model, args.model)
slug = args.model.replace("-", "_").replace("/", "_")

print(f"Loading {model_id}...")
tokenizer = AutoTokenizer.from_pretrained(model_id)
if mode == "semantic":
    model = AutoModel.from_pretrained(model_id)
else:
    model = AutoModelForTokenClassification.from_pretrained(model_id)
model.eval()

export_model = model
if "token_type_ids" not in inspect.signature(model.forward).parameters:
    print(f"{model_id}'s forward() has no token_type_ids param (e.g. MPNet) — wrapping to drop it.")
    export_model = _DropsTokenTypeIds(model)
    export_model.eval()

dummy_ids = torch.zeros(1, 16, dtype=torch.long)
dummy_mask = torch.ones(1, 16, dtype=torch.long)
dummy_tti = torch.zeros(1, 16, dtype=torch.long)

batch = torch.export.Dim("batch")
seq   = torch.export.Dim("seq", min=1, max=512)
dynamic_shapes = {
    "input_ids":      {0: batch, 1: seq},
    "attention_mask": {0: batch, 1: seq},
    "token_type_ids": {0: batch, 1: seq},
}

onnx_path = os.path.join(out_dir, f"{slug}.onnx")
print(f"Exporting ONNX to {onnx_path}...")

if mode == "semantic":
    torch.onnx.export(
        export_model,
        (dummy_ids, dummy_mask, dummy_tti),
        onnx_path,
        input_names=["input_ids", "attention_mask", "token_type_ids"],
        output_names=["last_hidden_state"],
        dynamic_shapes=dynamic_shapes,
        opset_version=18,
    )
else:
    torch.onnx.export(
    export_model,
    (dummy_ids, dummy_mask, dummy_tti),
    onnx_path,
    input_names=["input_ids", "attention_mask", "token_type_ids"],
    output_names=["logits"],
    dynamic_axes={
        "input_ids":      {0: "batch", 1: "seq"},
        "attention_mask": {0: "batch", 1: "seq"},
        "token_type_ids": {0: "batch", 1: "seq"},
        "logits":         {0: "batch", 1: "seq"},
    },
    opset_version=14,
)

print("ONNX export done.")

vocab_path = os.path.join(out_dir, f"{slug}_vocab.txt")
vocab_url = f"https://huggingface.co/{model_id}/resolve/main/vocab.txt"
print(f"Downloading vocab.txt...")
r = requests.get(vocab_url)
r.raise_for_status()
with open(vocab_path, "wb") as f:
    f.write(r.content)
print(f"vocab saved to {vocab_path}")

if mode == "pos":
    labels_path = os.path.join(out_dir, f"{slug}_labels.json")
    with open(labels_path, "w") as f:
        json.dump({str(k): v for k, v in sorted(model.config.id2label.items())}, f)
    print(f"labels saved to {labels_path}")
    print(f"\nDone. Files: {onnx_path}, {vocab_path}, {labels_path}")
else:
    print(f"\nDone. Files: {onnx_path}, {vocab_path}")