#!/usr/bin/env python
"""
Export a POS-tagging model to ONNX.

Supported models:
  mobilebert  mrm8488/mobilebert-finetuned-pos  (fast, ~100 MB)
  bert-base   QCRI/bert-base-multilingual-cased-pos-english  (~400 MB)

Usage:
    python export_model.py [output_dir] [--model mobilebert|bert-base]
"""
import sys
import os
import argparse
import json

# Force UTF-8 stdout/stderr so torch's emoji output doesn't crash on Windows cp1252.
if sys.stdout.encoding != 'utf-8':
    sys.stdout.reconfigure(encoding='utf-8', errors='replace')
if sys.stderr.encoding != 'utf-8':
    sys.stderr.reconfigure(encoding='utf-8', errors='replace')

try:
    import torch
    from transformers import AutoTokenizer, AutoModelForTokenClassification
    import requests
except ImportError as e:
    print(f"Missing dependency: {e}")
    print("pip install transformers torch onnx onnxscript requests")
    sys.exit(1)

MODELS = {
    "mobilebert": "mrm8488/mobilebert-finetuned-pos",
    "bert-base":  "QCRI/bert-base-multilingual-cased-pos-english",
}

parser = argparse.ArgumentParser()
parser.add_argument("out_dir", nargs="?", default=os.path.dirname(os.path.abspath(__file__)))
parser.add_argument("--model", choices=list(MODELS.keys()), default="mobilebert")
args = parser.parse_args()

model_id = MODELS[args.model]
out_dir = args.out_dir
slug = args.model.replace("-", "_")

print(f"Loading {model_id}...")
tokenizer = AutoTokenizer.from_pretrained(model_id)
model = AutoModelForTokenClassification.from_pretrained(model_id)
model.eval()

dummy_ids = torch.zeros(1, 16, dtype=torch.long)
dummy_mask = torch.ones(1, 16, dtype=torch.long)
dummy_tti = torch.zeros(1, 16, dtype=torch.long)

onnx_path = os.path.join(out_dir, f"{slug}_pos.onnx")
print(f"Exporting ONNX to {onnx_path}...")
torch.onnx.export(
    model,
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

labels_path = os.path.join(out_dir, f"{slug}_labels.json")
with open(labels_path, "w") as f:
    json.dump({str(k): v for k, v in sorted(model.config.id2label.items())}, f)
print(f"labels saved to {labels_path}")

print(f"\nDone. Files: {onnx_path}, {vocab_path}, {labels_path}")
