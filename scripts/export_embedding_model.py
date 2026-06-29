#!/usr/bin/env python
"""
Export a sentence-transformer model as an ONNX embedding extractor for semantic mode.

Variants:
  minilm  sentence-transformers/all-MiniLM-L6-v2   22 MB, 384-dim, 6-layer
  mpnet   sentence-transformers/all-mpnet-base-v2  110 MB, 768-dim, 12-layer MPNet
          (comparable to BERT-base; top of SBERT benchmarks at that scale)

Usage:
    python export_embedding_model.py [--variant minilm|mpnet] [output_dir]
"""
import sys
import os
import argparse

if sys.stdout.encoding != 'utf-8':
    sys.stdout.reconfigure(encoding='utf-8', errors='replace')
if sys.stderr.encoding != 'utf-8':
    sys.stderr.reconfigure(encoding='utf-8', errors='replace')

try:
    import torch
    from transformers import AutoModel, AutoTokenizer
    import requests
except ImportError as e:
    print(f"Missing dependency: {e}")
    print("pip install transformers torch onnx onnxscript requests")
    sys.exit(1)

VARIANTS = {
    "minilm": {
        "model_id": "sentence-transformers/all-MiniLM-L6-v2",
        "size_note": "~22 MB",
        "onnx_name": "minilm_embed.onnx",
        "vocab_name": "minilm_vocab.txt",
    },
    "mpnet": {
        "model_id": "sentence-transformers/all-mpnet-base-v2",
        "size_note": "~110 MB",
        "onnx_name": "mpnet_embed.onnx",
        "vocab_name": "mpnet_vocab.txt",
    },
}

def default_data_dir():
    if sys.platform == "win32":
        base = os.environ.get("APPDATA") or os.path.join(os.path.expanduser("~"), "AppData", "Roaming")
    elif sys.platform == "darwin":
        base = os.path.join(os.path.expanduser("~"), "Library", "Application Support")
    else:
        base = os.environ.get("XDG_CONFIG_HOME") or os.path.join(os.path.expanduser("~"), ".config")
    return os.path.join(base, "natural-syntax-ls")

parser = argparse.ArgumentParser()
parser.add_argument("--variant", choices=list(VARIANTS), default="mpnet",
                    help="Which model to export (default: mpnet)")
parser.add_argument("out_dir", nargs="?", default=None,
                    help="Output directory (default: platform config dir for natural-syntax-ls)")
args = parser.parse_args()

v = VARIANTS[args.variant]
out_dir = args.out_dir or default_data_dir()
os.makedirs(out_dir, exist_ok=True)

print(f"Loading {v['model_id']} ({v['size_note']})...")
tokenizer = AutoTokenizer.from_pretrained(v["model_id"])
model = AutoModel.from_pretrained(v["model_id"])
model.eval()

dummy_ids  = torch.zeros(1, 16, dtype=torch.long)
dummy_mask = torch.ones(1, 16, dtype=torch.long)
dummy_tti  = torch.zeros(1, 16, dtype=torch.long)

batch = torch.export.Dim("batch")
seq   = torch.export.Dim("seq", min=1, max=512)
dynamic_shapes = {
    "input_ids":      {0: batch, 1: seq},
    "attention_mask": {0: batch, 1: seq},
    "token_type_ids": {0: batch, 1: seq},
}

onnx_path = os.path.join(out_dir, v["onnx_name"])
print(f"Exporting ONNX to {onnx_path} ...")
torch.onnx.export(
    model,
    (dummy_ids, dummy_mask, dummy_tti),
    onnx_path,
    input_names=["input_ids", "attention_mask", "token_type_ids"],
    output_names=["last_hidden_state"],
    dynamic_shapes=dynamic_shapes,
    opset_version=18,
)
print("ONNX export done.")

vocab_path = os.path.join(out_dir, v["vocab_name"])
vocab_url  = f"https://huggingface.co/{v['model_id']}/resolve/main/vocab.txt"
print("Downloading vocab.txt ...")
r = requests.get(vocab_url)
r.raise_for_status()
with open(vocab_path, "wb") as f:
    f.write(r.content)
print(f"vocab saved to {vocab_path}")

print(f"\nDone. Files: {onnx_path}, {vocab_path}")
print("\nUsage:")
print(f"  natural-syntax-ls --mode semantic --embed-variant {args.variant} --embed-model {onnx_path} --vocab {vocab_path}")
