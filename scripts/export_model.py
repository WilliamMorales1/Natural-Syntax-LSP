#!/usr/bin/env python
"""
Export a POS-tagging, sentence-transformer, or dependency-parsing model to ONNX.

Supported models:
  mobilebert  mrm8488/mobilebert-finetuned-pos  (fast, ~100 MB)
  bert-base   QCRI/bert-base-multilingual-cased-pos-english  (~400 MB)
  xlmr-es     wietsedv/xlm-roberta-base-ft-udpos28-es  Spanish UPOS (XLM-R base)
  xlmr-fr     wietsedv/xlm-roberta-base-ft-udpos28-fr  French UPOS (XLM-R base)
  xlmr-ru     wietsedv/xlm-roberta-base-ft-udpos28-ru  Russian UPOS (XLM-R base)
  multilingual-minilm
              sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2
              384-dim, 12-layer, 50+ languages (semantic mode)

--model values other than the names above are treated as a
diaparser (github.com/Unipisa/diaparser) catalog name and export a biaffine
UD dependency parser (default en_ewt.electra-base) as a 3-input/2-output
ONNX graph: input_ids, attention_mask, pool_matrix -> arc_logits, rel_logits.
See _export_dependency() below for why this needs a custom wrapper rather
than exporting the diaparser checkpoint directly.

Any Hugging Face repo id can be exported with an explicit --mode:
  --mode pos       token-classification model (e.g. wietsedv/xlm-roberta-base-ft-udpos28-es)
  --mode semantic  encoder used for sentence embeddings
Models with a SentencePiece Unigram tokenizer ship their HF tokenizer.json as
{slug}_tokenizer.json instead of a vocab.txt; WordPiece models also get their
tokenizer_config.json copied to {slug}_tokenizer.json when one exists.

--int8 quantizes the exported weights to int8 in place (about 4x smaller; the
Spanish dependency parser loses accuracy when quantized, so leave it fp32).

Usage:
    python export_model.py [output_dir] [--model <name from models.json>|<hf repo id>] [--mode pos|semantic|dependency] [--int8]
"""

import argparse
import io
import json
import os
import sys

# Force UTF-8 stdout/stderr so torch's emoji output doesn't crash on Windows cp1252.
# sys.stdout/stderr are typed as TextIO (no .reconfigure) but are actually
# TextIOWrapper at runtime whenever they're a real console/file stream.
if isinstance(sys.stdout, io.TextIOWrapper) and sys.stdout.encoding != "utf-8":
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
if isinstance(sys.stderr, io.TextIOWrapper) and sys.stderr.encoding != "utf-8":
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

try:
    import requests
    import torch
    from transformers import AutoModel, AutoModelForTokenClassification, AutoTokenizer
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


def slugify(name):
    """Return the model directory stem: - . / become _ (kept in step with internal/models, setup.sh, setup.ps1 and extension.js; see data/slug_cases.tsv)."""
    return name.replace("-", "_").replace(".", "_").replace("/", "_")


def _load_catalog():
    """Merge models.json from the repo, then the config dir; later entries replace same-named ones, as in the server."""
    here = os.path.dirname(os.path.abspath(__file__))
    cfg = os.environ.get("XDG_CONFIG_HOME", os.path.join(os.path.expanduser("~"), ".config"))
    merged, found = {}, False
    for d in (os.path.join(here, "..", "data"), os.path.join(cfg, "natural-syntax-ls")):
        path = os.path.join(d, "models.json")
        if os.path.exists(path):
            found = True
            with open(path, encoding="utf-8") as f:
                for m in json.load(f)["models"]:
                    merged[m["name"]] = m
    if not found:
        raise SystemExit("models.json not found next to the repo or in the config dir")
    return list(merged.values())


_CATALOG = _load_catalog()
POS_MODELS = {m["name"]: m["source"] for m in _CATALOG if m["mode"] == "pos"}
SEMANTIC_MODELS = {m["name"]: m["source"] for m in _CATALOG if m["mode"] == "semantic"}
MODELS = {**POS_MODELS, **SEMANTIC_MODELS}

# diaparser (github.com/Unipisa/diaparser) catalog name used when --model is
# omitted or isn't a known POS/semantic name.
DEFAULT_DEPENDENCY_MODEL = next(
    (m["source"] for m in _CATALOG if m["mode"] == "dependency" and m.get("default")),
    "en_ewt.electra-base",
)

# Dependency catalog entries whose source is a Hugging Face repo hold a diaparser checkpoint named after the entry.
DEPENDENCY_SOURCES = {m["name"]: m["source"] for m in _CATALOG if m["mode"] == "dependency"}

parser = argparse.ArgumentParser()
parser.add_argument(
    "out_dir",
    nargs="?",
    default=os.path.dirname(os.path.abspath(__file__)),
    help="Models root; each model is written to <out_dir>/<slug>/.",
)
parser.add_argument(
    "--model",
    default=None,
    help=f"One of {list(MODELS.keys())} (pos/semantic), "
    f"or a diaparser catalog name for dependency parsing "
    f"(default: {DEFAULT_DEPENDENCY_MODEL}).",
)
parser.add_argument(
    "--mode",
    choices=["pos", "semantic", "dependency"],
    default=None,
    help="Force the export mode; required for Hugging Face repo ids.",
)
parser.add_argument(
    "--int8",
    action="store_true",
    help="Quantize the exported weights to int8 in place.",
)
args = parser.parse_args()

if args.model in ("minilm", "mpnet") and args.mode is None:
    raise SystemExit(
        f"--model {args.model} was removed; use multilingual-minilm for semantic mode."
    )

out_dir = args.out_dir
os.makedirs(out_dir, exist_ok=True)

# Mode is inferred from --model: known POS/semantic names pick those modes;
# anything else (including no --model at all) falls through to pos's
# default bert-base, except when the name isn't a known model at all, which
# means it's a diaparser catalog name for dependency mode.
if args.mode is not None:
    mode = args.mode
elif args.model is None or args.model in POS_MODELS:
    mode = "pos"
elif args.model in SEMANTIC_MODELS:
    mode = "semantic"
else:
    mode = "dependency"


def _quantize_int8(onnx_path):
    """Replace onnx_path with a dynamically int8-quantized copy and drop its external data file."""
    try:
        from onnxruntime.quantization import QuantType, quantize_dynamic
    except ImportError as e:
        raise SystemExit(f"--int8 needs onnxruntime: {e}")
    tmp = onnx_path + ".int8.tmp"
    quantize_dynamic(
        onnx_path, tmp, weight_type=QuantType.QInt8, use_external_data_format=False
    )
    os.replace(tmp, onnx_path)
    data = onnx_path + ".data"
    if os.path.exists(data):
        os.remove(data)
    print(f"Quantized {onnx_path} to int8 ({os.path.getsize(onnx_path) / 1e6:.0f} MB)")


def _fetch(repo, name):
    r = requests.get(f"https://huggingface.co/{repo}/resolve/main/{name}")
    return r.content if r.status_code == 200 else None


def _write_tokenizer_files(repo, slug, out_dir):
    """Save the tokenizer the Go side needs next to the model: tokenizer.json for Unigram, else vocab.txt (+ tokenizer_config.json)."""
    tok_json = _fetch(repo, "tokenizer.json")
    if (
        tok_json is not None
        and json.loads(tok_json).get("model", {}).get("type") == "Unigram"
    ):
        path = os.path.join(out_dir, f"{slug}_tokenizer.json")
        with open(path, "wb") as f:
            f.write(tok_json)
        print(f"Unigram tokenizer saved to {path}")
        return [path]
    saved = []
    vocab = _fetch(repo, "vocab.txt")
    if vocab is None:
        raise SystemExit(
            f"{repo} has neither a Unigram tokenizer.json nor a WordPiece vocab.txt; unsupported tokenizer."
        )
    path = os.path.join(out_dir, f"{slug}_vocab.txt")
    with open(path, "wb") as f:
        f.write(vocab)
    print(f"vocab saved to {path}")
    saved.append(path)
    cfg = _fetch(repo, "tokenizer_config.json")
    if cfg is not None:
        path = os.path.join(out_dir, f"{slug}_tokenizer.json")
        with open(path, "wb") as f:
            f.write(cfg)
        print(f"tokenizer config saved to {path}")
        saved.append(path)
    return saved


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
    from diaparser.parsers import Parser
    from torch import nn

    source = DEPENDENCY_SOURCES.get(model_name, model_name)
    if "/" in source and not os.path.exists(source):
        from huggingface_hub import hf_hub_download

        source = hf_hub_download(source, model_name)
    if os.path.exists(source):
        # A local checkpoint pickles diaparser classes, which recent torch refuses to load by default.
        _load = torch.load
        torch.load = lambda *a, **k: _load(*a, **{**k, "weights_only": False})
    print(f"Loading diaparser model {model_name}...")
    p = Parser.load(source)
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
                sd[f"weight_ih_l{i}_reverse"] = (
                    old.b_cells[i].weight_ih.detach().clone()
                )
                sd[f"weight_hh_l{i}_reverse"] = (
                    old.b_cells[i].weight_hh.detach().clone()
                )
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
            out = self.bert(
                input_ids, attention_mask=attention_mask, output_hidden_states=True
            )
            hs = torch.stack(out.hidden_states[-self.n_layers :], 0)
            mixed = self.scalar_mix(hs)  # [1, n_sub, hidden]
            word_embed = torch.matmul(pool_matrix, mixed)  # [1, seq, hidden]
            x, _ = self.lstm(word_embed)  # [1, seq, 2*hidden]
            arc_d = self.mlp_arc_d(x)
            arc_h = self.mlp_arc_h(x)
            rel_d = self.mlp_rel_d(x)
            rel_h = self.mlp_rel_h(x)
            s_arc = self._biaffine(
                arc_d, arc_h, self.arc_weight, self.arc_bias_x, self.arc_bias_y
            )  # [1, seq, seq]
            s_rel = self._biaffine(
                rel_d, rel_h, self.rel_weight, self.rel_bias_x, self.rel_bias_y
            )  # [1, n_rels, seq, seq]
            s_rel = s_rel.permute(0, 2, 3, 1)  # [1, seq, seq, n_rels]
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
    dummy_pool = torch.tensor(
        [
            [
                [1.0, 0.0, 0.0, 0.0],
                [0.0, 1.0, 0.0, 0.0],
                [0.0, 0.0, 0.5, 0.5],
            ]
        ],
        dtype=torch.float32,
    )

    slug = slugify(model_name)
    out_dir = os.path.join(out_dir, f"{slug}_dependency")
    os.makedirs(out_dir, exist_ok=True)
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
    if args.int8:
        _quantize_int8(onnx_path)

    bert_model_id = m.feat_embed.model
    print(f"Fetching tokenizer from {bert_model_id}...")
    vocab_path = ", ".join(
        _write_tokenizer_files(bert_model_id, f"{slug}_dependency", out_dir)
    )

    rel_vocab = p.transform.DEPREL.vocab
    rels = [rel_vocab.itos[i] for i in range(len(rel_vocab))]
    rels_path = os.path.join(out_dir, f"{slug}_dependency_rels.json")
    with open(rels_path, "w") as f:
        json.dump(rels, f)
    print(
        f"rel labels saved to {rels_path} (index 0, {rels[0]!r}, is an unused placeholder)"
    )

    print(f"\nDone. Files: {onnx_path}, {vocab_path}, {rels_path}")
    print(
        f'Set naturalSyntaxLs.model to {model_name!r} (with naturalSyntaxLs.mode = "dependency").'
    )


if mode == "dependency":
    _export_dependency(args.model or DEFAULT_DEPENDENCY_MODEL, out_dir)
    sys.exit(0)

if args.model is None:
    args.model = "bert-base"
model_id = MODELS.get(args.model, args.model)
slug = slugify(args.model)
out_dir = os.path.join(out_dir, slug)
os.makedirs(out_dir, exist_ok=True)

print(f"Loading {model_id}...")
tokenizer = AutoTokenizer.from_pretrained(model_id)
if mode == "semantic":
    model = AutoModel.from_pretrained(model_id)
else:
    model = AutoModelForTokenClassification.from_pretrained(model_id)
model.eval()

export_model = model
if "token_type_ids" not in inspect.signature(model.forward).parameters:
    print(
        f"{model_id}'s forward() has no token_type_ids param (e.g. MPNet) — wrapping to drop it."
    )
    export_model = _DropsTokenTypeIds(model)
    export_model.eval()

dummy_ids = torch.zeros(1, 16, dtype=torch.long)
dummy_mask = torch.ones(1, 16, dtype=torch.long)
dummy_tti = torch.zeros(1, 16, dtype=torch.long)

batch = torch.export.Dim("batch")
seq = torch.export.Dim("seq", min=1, max=512)
dynamic_shapes = {
    "input_ids": {0: batch, 1: seq},
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
            "input_ids": {0: "batch", 1: "seq"},
            "attention_mask": {0: "batch", 1: "seq"},
            "token_type_ids": {0: "batch", 1: "seq"},
            "logits": {0: "batch", 1: "seq"},
        },
        opset_version=14,
    )

print("ONNX export done.")
if args.int8:
    _quantize_int8(onnx_path)

tokenizer_files = _write_tokenizer_files(model_id, slug, out_dir)
vocab_path = ", ".join(tokenizer_files)

if mode == "pos":
    labels_path = os.path.join(out_dir, f"{slug}_labels.json")
    with open(labels_path, "w") as f:
        json.dump({str(k): v for k, v in sorted(model.config.id2label.items())}, f)
    print(f"labels saved to {labels_path}")
    print(f"\nDone. Files: {onnx_path}, {vocab_path}, {labels_path}")
else:
    print(f"\nDone. Files: {onnx_path}, {vocab_path}")
