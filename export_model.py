#!/usr/bin/env python3
"""
Export mrm8488/mobilebert-finetuned-pos to ONNX and download vocab.txt.

Requirements:
    pip install transformers torch onnx

Usage:
    python export_model.py [output_dir]
    # output_dir defaults to the directory of this script
"""
import sys
import os

try:
    import torch
    from transformers import AutoTokenizer, AutoModelForTokenClassification
    import requests
except ImportError as e:
    print(f"Missing dependency: {e}")
    print("pip install transformers torch onnx requests")
    sys.exit(1)

MODEL_ID = "mrm8488/mobilebert-finetuned-pos"
out_dir = sys.argv[1] if len(sys.argv) > 1 else os.path.dirname(os.path.abspath(__file__))

print(f"Loading {MODEL_ID}...")
tokenizer = AutoTokenizer.from_pretrained(MODEL_ID)
model = AutoModelForTokenClassification.from_pretrained(MODEL_ID)
model.eval()

# Dummy input for tracing (batch=1, seq=16).
dummy_ids = torch.zeros(1, 16, dtype=torch.long)
dummy_mask = torch.ones(1, 16, dtype=torch.long)
dummy_tti = torch.zeros(1, 16, dtype=torch.long)

onnx_path = os.path.join(out_dir, "mobilebert_pos.onnx")
print(f"Exporting ONNX to {onnx_path}...")
torch.onnx.export(
    model,
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

vocab_path = os.path.join(out_dir, "vocab.txt")
vocab_url = f"https://huggingface.co/{MODEL_ID}/resolve/main/vocab.txt"
print(f"Downloading vocab.txt...")
r = requests.get(vocab_url)
r.raise_for_status()
with open(vocab_path, "wb") as f:
    f.write(r.content)
print(f"vocab.txt saved to {vocab_path}")

print("\nDone. Place mobilebert_pos.onnx and vocab.txt next to the natural-syntax-ls binary.")
print("Also place onnxruntime.dll (Windows) / libonnxruntime.so (Linux) next to the binary.")
print("Download ORT from: https://github.com/microsoft/onnxruntime/releases")
