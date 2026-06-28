#!/usr/bin/env python
"""
Export sentence-transformers/all-MiniLM-L6-v2 as an ONNX embedding extractor
for semantic mode.

This model is 22 MB and was trained with contrastive learning specifically for
semantic similarity — far better than raw BERT hidden states for clustering.
It outputs last_hidden_state ([batch, seq, 384]); the Go server mean-pools
subword vectors per word and runs k-means to assign semantic colours.

Usage:
    python export_embedding_model.py [output_dir]
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

MODEL_ID = "sentence-transformers/all-MiniLM-L6-v2"

parser = argparse.ArgumentParser()
parser.add_argument("out_dir", nargs="?", default=os.path.dirname(os.path.abspath(__file__)))
args = parser.parse_args()
out_dir = args.out_dir

print(f"Loading {MODEL_ID} (~22 MB)...")
tokenizer = AutoTokenizer.from_pretrained(MODEL_ID)
model = AutoModel.from_pretrained(MODEL_ID)
model.eval()

dummy_ids  = torch.zeros(1, 16, dtype=torch.long)
dummy_mask = torch.ones(1, 16, dtype=torch.long)
dummy_tti  = torch.zeros(1, 16, dtype=torch.long)

onnx_path = os.path.join(out_dir, "minilm_embed.onnx")
print(f"Exporting ONNX to {onnx_path} ...")
torch.onnx.export(
    model,
    (dummy_ids, dummy_mask, dummy_tti),
    onnx_path,
    input_names=["input_ids", "attention_mask", "token_type_ids"],
    output_names=["last_hidden_state"],
    dynamic_axes={
        "input_ids":         {0: "batch", 1: "seq"},
        "attention_mask":    {0: "batch", 1: "seq"},
        "token_type_ids":    {0: "batch", 1: "seq"},
        "last_hidden_state": {0: "batch", 1: "seq"},
    },
    opset_version=14,
)
print("ONNX export done.")

# all-MiniLM-L6-v2 uses the bert-base-uncased WordPiece vocab (30522 tokens).
vocab_path = os.path.join(out_dir, "minilm_vocab.txt")
vocab_url  = f"https://huggingface.co/{MODEL_ID}/resolve/main/vocab.txt"
print("Downloading vocab.txt ...")
r = requests.get(vocab_url)
r.raise_for_status()
with open(vocab_path, "wb") as f:
    f.write(r.content)
print(f"vocab saved to {vocab_path}")

print(f"\nDone. Files: {onnx_path}, {vocab_path}")
print("\nUsage:")
print(f"  natural-syntax-ls --mode semantic --embed-model {onnx_path} --vocab {vocab_path}")
