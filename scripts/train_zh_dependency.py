#!/usr/bin/env python
"""
Train a Chinese diaparser dependency model on UD Chinese-GSD and GSDSimp.

diaparser ships no Chinese parser and Chinese has no spaces, so the parser works on
characters and learns word boundaries itself. Every unit the server's tokenizer would
emit (one Han character, one punctuation mark, or one run of letters/digits) is a
parser token. A multi-unit gold word links each unit but the last to that last unit
with the relation "wp"; the last unit carries the word's real head and relation.
The server merges "wp" chains back into words (see DependencyModel in Go).

    python train_zh_dependency.py WORKDIR [--epochs 30] [--bert hfl/chinese-roberta-wwm-ext]

Needs a GPU and diaparser (pip install --no-deps diaparser nltk stanza). With transformers 5, first
replace "tokenizer.max_len" by "tokenizer.model_max_length" in diaparser/parsers/*.py. Word embeddings
are off and BERT is not projected, so the model matches what export_model.py can export. Output: WORKDIR/zh_ud.mbert (load with diaparser Parser.load, export with
export_model.py --mode dependency --model WORKDIR/zh_ud.mbert).
"""

import argparse
import os
import subprocess
import sys
import unicodedata
import urllib.request

BASE = "https://raw.githubusercontent.com/UniversalDependencies/"
FILES = {
    "gsd": "UD_Chinese-GSD/master/zh_gsd-ud-{}.conllu",
    "gsdsimp": "UD_Chinese-GSDSimp/master/zh_gsdsimp-ud-{}.conllu",
}
WORD_PART = "wp"


def is_cjk(c):
    return c == "ー" or any(
        unicodedata.name(c, "").startswith(p) for p in ("CJK UNIFIED", "HIRAGANA", "KATAKANA", "CJK COMPATIBILITY IDEOGRAPH")
    )


def units(text):
    """Split text like the Go tokenizer with per-character CJK; returns (unit, begin, end) triples."""
    out = []
    i, n = 0, len(text)
    while i < n:
        c = text[i]
        if c.isspace():
            i += 1
            continue
        cat = unicodedata.category(c)
        if is_cjk(c) or cat[0] in "PS":
            out.append((c, i, i + 1))
            i += 1
            continue
        j = i
        while j < n and not text[j].isspace() and not is_cjk(text[j]) and unicodedata.category(text[j])[0] not in "PS":
            j += 1
        out.append((text[i:j], i, j))
        i = j
    return out


def read_conllu(path):
    sent = []
    for line in open(path, encoding="utf-8"):
        line = line.rstrip("\n")
        if not line:
            if sent:
                yield sent
            sent = []
        elif line[0] != "#":
            c = line.split("\t")
            if "-" in c[0] or "." in c[0]:
                continue
            sent.append((int(c[0]), c[1], int(c[6]), c[7].split(":")[0]))
    if sent:
        yield sent


def to_chars(sent):
    """Rewrite a word-level sentence as unit-level rows (form, head, rel); None if a unit straddles two words."""
    text = "".join(f for _, f, _, _ in sent)
    spans, pos = [], 0
    for _, form, _, _ in sent:
        spans.append((pos, pos + len(form)))
        pos += len(form)
    word_of = {}
    for wi, (b, e) in enumerate(spans):
        for k in range(b, e):
            word_of[k] = wi
    us = units(text)
    members = [[] for _ in sent]
    for ui, (_, b, e) in enumerate(us):
        if len({word_of[k] for k in range(b, e)}) != 1:
            return None
        members[word_of[b]].append(ui)
    if any(not m for m in members):
        return None
    last = {sent[wi][0]: m[-1] + 1 for wi, m in enumerate(members)}
    rows = [("", 0, "")] * len(us)
    for wi, m in enumerate(members):
        _, _, head, rel = sent[wi]
        for ui in m[:-1]:
            rows[ui] = (us[ui][0], m[-1] + 1, WORD_PART)
        rows[m[-1]] = (us[m[-1]][0], 0 if head == 0 else last[head], rel)
    return rows


def write_split(out, names, split):
    kept = dropped = 0
    with open(out, "w", encoding="utf-8") as f:
        for name in names:
            path = os.path.join(os.path.dirname(out), os.path.basename(FILES[name].format(split)))
            if not os.path.exists(path):
                urllib.request.urlretrieve(BASE + FILES[name].format(split), path)
            for sent in read_conllu(path):
                rows = to_chars(sent)
                if rows is None:
                    dropped += 1
                    continue
                kept += 1
                for i, (form, head, rel) in enumerate(rows, 1):
                    f.write(f"{i}\t{form}\t_\t_\t_\t_\t{head}\t{rel}\t_\t_\n")
                f.write("\n")
    print(f"{out}: {kept} sentences, {dropped} dropped")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("workdir")
    ap.add_argument("--bert", default="hfl/chinese-roberta-wwm-ext")
    ap.add_argument("--epochs", default="30")
    ap.add_argument("--patience", default="8")
    ap.add_argument("--device", default="0")
    ap.add_argument("--prepare-only", action="store_true")
    args = ap.parse_args()
    os.makedirs(args.workdir, exist_ok=True)
    for split in ("train", "dev", "test"):
        write_split(os.path.join(args.workdir, f"zh-{split}.conllu"), ["gsd", "gsdsimp"], split)
    if args.prepare_only:
        return
    w = args.workdir
    conf = os.path.join(w, "train.ini")
    with open(conf, "w") as f:
        f.write(f"[run]\nepochs = {args.epochs}\npatience = {args.patience}\nn_word_embed = 0\nn_feat_embed = 768\n")
    subprocess.check_call(
        [
            sys.executable, "-m", "diaparser.cmds.biaffine_dependency",
            "-c", conf, "-d", args.device, "-p", os.path.join(w, "zh_ud.mbert"), "train",
            "-b", "-f", "bert", "--bert", args.bert,
            "--train", f"{w}/zh-train.conllu", "--dev", f"{w}/zh-dev.conllu", "--test", f"{w}/zh-test.conllu",
        ]
    )


if __name__ == "__main__":
    main()
