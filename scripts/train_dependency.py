#!/usr/bin/env python
"""
Train a diaparser dependency model for a language diaparser ships none for: zh (UD Chinese-GSD
and GSDSimp) or pt (UD Portuguese-Bosque and GSD).

The parser tokens are the units the server's tokenizer emits: one Han character, one
punctuation mark or symbol, or one run of other characters between spaces. A gold word
of several units links each unit but the last to that last unit with the relation "wp";
the last unit carries the word's real head and relation. The server merges "wp" chains
back into words (see DependencyModel in Go), so Chinese needs no segmenter.

Portuguese contractions (do = de + o, dá-lo = dá + lo) are UD multiword tokens. The
server sees only the written form, so each one becomes one word that takes the head and
relation of its part whose head lies outside it.

    python train_dependency.py WORKDIR [--lang zh|pt] [--epochs 30] [--bert MODEL]

Needs a GPU and diaparser (pip install --no-deps diaparser nltk "stanza==1.10.1" emoji; newer stanza needs udtools). With transformers 5, first
replace "tokenizer.max_len" by "tokenizer.model_max_length" in diaparser/parsers/*.py. Word embeddings
are off and BERT is not projected, so the model matches what export_model.py can export. Output: WORKDIR/zh_ud.mbert (load with diaparser Parser.load, export with
export_model.py --mode dependency --model WORKDIR/zh_ud.mbert); pt writes WORKDIR/pt_ud.bertimbau.
"""

import argparse
import os
import subprocess
import sys
import unicodedata
import urllib.request

BASE = "https://raw.githubusercontent.com/UniversalDependencies/"
LANGS = {
    "zh": {
        "files": ["UD_Chinese-GSD/master/zh_gsd-ud-{}.conllu", "UD_Chinese-GSDSimp/master/zh_gsdsimp-ud-{}.conllu"],
        "bert": "hfl/chinese-roberta-wwm-ext",
        "name": "zh_ud.mbert",
        "sep": "",  # gold words are written without spaces
    },
    "pt": {
        "files": ["UD_Portuguese-Bosque/master/pt_bosque-ud-{}.conllu", "UD_Portuguese-GSD/master/pt_gsd-ud-{}.conllu"],
        "bert": "neuralmind/bert-base-portuguese-cased",
        "name": "pt_ud.bertimbau",
        "sep": " ",
    },
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
    """Yield sentences as (id, form, head, rel) over written words, multiword tokens merged."""
    rows, mwts = [], []
    for line in open(path, encoding="utf-8"):
        line = line.rstrip("\n")
        if not line:
            if rows:
                yield merge_mwts(rows, mwts)
            rows, mwts = [], []
        elif line[0] != "#":
            c = line.split("\t")
            if "." in c[0]:
                continue
            if "-" in c[0]:
                a, b = map(int, c[0].split("-"))
                mwts.append((a, b, c[1]))
                continue
            rows.append((int(c[0]), c[1], int(c[6]), c[7].split(":")[0]))
    if rows:
        yield merge_mwts(rows, mwts)


def merge_mwts(rows, mwts):
    """Collapse each multiword token into one word headed like its part whose head lies outside it."""
    if not mwts:
        return rows
    span_of = {}
    for a, b, form in mwts:
        for i in range(a, b + 1):
            span_of[i] = (a, b, form)
    by_id = {r[0]: r for r in rows}
    words, new_id = [], {}
    for wid, form, head, rel in rows:
        span = span_of.get(wid)
        if span and wid != span[0]:
            new_id[wid] = len(words)
            continue
        if span:
            a, b, mform = span
            parts = [by_id[i] for i in range(a, b + 1)]
            outside = [p for p in parts if not a <= p[2] <= b]
            pick = next((p for p in outside if p[2] == 0), outside[0] if outside else parts[-1])
            form, head, rel = mform, pick[2], pick[3]
        words.append([form, head, rel])
        new_id[wid] = len(words)
    return [(i + 1, f, new_id.get(h, 0) if h else 0, r) for i, (f, h, r) in enumerate(words)]


def to_chars(sent, sep):
    """Rewrite a word-level sentence as unit-level rows (form, head, rel); None if a unit straddles two words."""
    text = sep.join(f for _, f, _, _ in sent)
    spans, pos = [], 0
    for _, form, _, _ in sent:
        spans.append((pos, pos + len(form)))
        pos += len(form) + len(sep)
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


def write_split(out, lang, split):
    kept = dropped = 0
    with open(out, "w", encoding="utf-8") as f:
        for tmpl in lang["files"]:
            path = os.path.join(os.path.dirname(out), os.path.basename(tmpl.format(split)))
            if not os.path.exists(path):
                urllib.request.urlretrieve(BASE + tmpl.format(split), path)
            for sent in read_conllu(path):
                rows = to_chars(sent, lang["sep"])
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
    ap.add_argument("--lang", default="zh", choices=sorted(LANGS))
    ap.add_argument("--bert", help="encoder (default: per language)")
    ap.add_argument("--epochs", default="30")
    ap.add_argument("--patience", default="8")
    ap.add_argument("--device", default="0")
    ap.add_argument("--prepare-only", action="store_true")
    args = ap.parse_args()
    lang = LANGS[args.lang]
    code = args.lang
    os.makedirs(args.workdir, exist_ok=True)
    for split in ("train", "dev", "test"):
        write_split(os.path.join(args.workdir, f"{code}-{split}.conllu"), lang, split)
    if args.prepare_only:
        return
    w = args.workdir
    conf = os.path.join(w, "train.ini")
    with open(conf, "w") as f:
        f.write(f"[run]\nepochs = {args.epochs}\npatience = {args.patience}\nn_word_embed = 0\nn_feat_embed = 768\n")
    subprocess.check_call(
        [
            sys.executable, "-m", "diaparser.cmds.biaffine_dependency",
            "-c", conf, "-d", args.device, "-p", os.path.join(w, lang["name"]), "train",
            "-b", "-f", "bert", "--bert", args.bert or lang["bert"],
            "--train", f"{w}/{code}-train.conllu", "--dev", f"{w}/{code}-dev.conllu", "--test", f"{w}/{code}-test.conllu",
        ]
    )


if __name__ == "__main__":
    main()
