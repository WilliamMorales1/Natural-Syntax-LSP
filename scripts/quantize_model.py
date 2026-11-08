#!/usr/bin/env python
"""Dynamic int8 quantization of an exported ONNX model: quantize_model.py IN.onnx OUT.onnx. Sidecar files must be copied by hand."""

import sys

from onnxruntime.quantization import QuantType, quantize_dynamic

quantize_dynamic(
    sys.argv[1],
    sys.argv[2],
    weight_type=QuantType.QInt8,
    use_external_data_format=False,
)
