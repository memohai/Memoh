"""Sparse encoding Flask service using OpenSearch neural sparse model."""

import json
import os
import sys
from pathlib import Path

import torch
from flask import Flask, jsonify, request
from huggingface_hub import hf_hub_download
from transformers import AutoModelForMaskedLM, AutoTokenizer

DEFAULT_MODEL_REPO = "opensearch-project/opensearch-neural-sparse-encoding-multilingual-v1"
DEFAULT_PORT = 8085
DEFAULT_CACHE_DIR = os.environ.get(
    "SPARSE_CACHE_DIR",
    str(Path(__file__).resolve().parent / "hf-cache"),
)

model_repo = DEFAULT_MODEL_REPO
cache_dir = DEFAULT_CACHE_DIR
port = int(os.environ.get("SPARSE_PORT", DEFAULT_PORT))

app = Flask(__name__)

_model = None
_tokenizer = None
_idf = None
_special_token_ids: list[int] = []
def _load_model() -> None:
    global _model, _tokenizer, _idf, _special_token_ids
    Path(cache_dir).mkdir(parents=True, exist_ok=True)
    _model = AutoModelForMaskedLM.from_pretrained(model_repo, cache_dir=cache_dir)
    _tokenizer = AutoTokenizer.from_pretrained(model_repo, cache_dir=cache_dir)
    _model.eval()
    _idf = _load_idf(_tokenizer)
    _special_token_ids = [
        _tokenizer.vocab[tok]
        for tok in _tokenizer.special_tokens_map.values()
        if tok in _tokenizer.vocab
    ]


def _load_idf(tokenizer):
    local_path = hf_hub_download(
        repo_id=model_repo, filename="idf.json", cache_dir=cache_dir
    )
    with open(local_path, encoding="utf-8") as f:
        idf_data = json.load(f)
    idf_vector = [0.0] * tokenizer.vocab_size
    for tok, weight in idf_data.items():
        tid = tokenizer._convert_token_to_id_with_added_voc(tok)
        idf_vector[tid] = weight
    return torch.tensor(idf_vector)


@torch.no_grad()
def _encode_document(text: str) -> dict:
    feat = _tokenizer(
        [text],
        padding=True,
        truncation=True,
        return_tensors="pt",
        return_token_type_ids=False,
    )
    out = _model(**feat)[0]
    vals, _ = torch.max(out * feat["attention_mask"].unsqueeze(-1), dim=1)
    vals = torch.log(1 + torch.log(1 + torch.relu(vals)))
    vals[:, _special_token_ids] = 0
    return _sparse_to_dict(vals[0])


@torch.no_grad()
def _encode_documents(texts: list[str]) -> list[dict]:
    feat = _tokenizer(
        texts,
        padding=True,
        truncation=True,
        return_tensors="pt",
        return_token_type_ids=False,
    )
    out = _model(**feat)[0]
    vals, _ = torch.max(out * feat["attention_mask"].unsqueeze(-1), dim=1)
    vals = torch.log(1 + torch.log(1 + torch.relu(vals)))
    vals[:, _special_token_ids] = 0
    return [_sparse_to_dict(vals[i]) for i in range(vals.shape[0])]


def _encode_query(text: str) -> dict:
    feat = _tokenizer(
        [text],
        padding=True,
        truncation=True,
        return_tensors="pt",
        return_token_type_ids=False,
    )
    input_ids = feat["input_ids"]
    batch_size = input_ids.shape[0]
    qv = torch.zeros(batch_size, _tokenizer.vocab_size)
    qv[torch.arange(batch_size).unsqueeze(-1), input_ids] = 1
    sparse_vector = qv * _idf
    return _sparse_to_dict(sparse_vector[0])


def _sparse_to_dict(vector: torch.Tensor) -> dict:
    nz = torch.nonzero(vector, as_tuple=True)[0]
    return {"indices": nz.tolist(), "values": vector[nz].tolist()}


@app.route("/health", methods=["GET"])
def health():
    return jsonify(status="ok", model_loaded=True, model_repo=model_repo)


@app.route("/encode/document", methods=["POST"])
def encode_document():
    body = request.get_json(silent=True) or {}
    text = body.get("text", "")
    if not text:
        return jsonify(error="text is required"), 400
    return jsonify(_encode_document(text))


@app.route("/encode/query", methods=["POST"])
def encode_query():
    body = request.get_json(silent=True) or {}
    text = body.get("text", "")
    if not text:
        return jsonify(error="text is required"), 400
    return jsonify(_encode_query(text))


@app.route("/encode/documents", methods=["POST"])
def encode_documents():
    body = request.get_json(silent=True) or {}
    texts = body.get("texts", [])
    if not texts:
        return jsonify(error="texts is required"), 400
    return jsonify(_encode_documents(texts))


def main():
    print(f"[sparse-service] loading model {model_repo}...", file=sys.stderr, flush=True)
    _load_model()
    print(f"[sparse-service] listening on port {port}", file=sys.stderr, flush=True)
    app.run(host="0.0.0.0", port=port, threaded=True)


if __name__ == "__main__":
    main()
