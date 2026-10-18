# S8 · Retrieval validation (memalyze v0.1)

What we compared, on 60 Chinese "memories" and 16 queries (12 semantic rewrites + 4 exact terms):

| variant | recall@3 | MRR@10 |
|---|---|---|
| vector: qwen3-embedding:0.6b (1024d) | **0.94** | **0.92** |
| rrf(qwen3, bigram) | 0.94 | 0.87 |
| fts-bigram (CJK bigram pre-tokenized FTS5) | **0.88** | **0.81** |
| vector: nomic-embed-text (768d) | 0.47 | 0.46 |
| vector: nomic + nomic prefixes | 0.41 | 0.45 |
| fts-unicode61 (FTS5 default) | 0.19 | 0.19 |
| fts-trigram | 0.19 | 0.19 |

Conclusions that changed the design:

1. **FTS5's built-in tokenizers do not work for Chinese.** `unicode61` treats a whole
   sentence as one token (only Latin words match); `trigram` needs ≥3 chars so two-char
   terms like "审计" can never match. → pre-tokenize CJK into overlapping bigrams in Go
   (`internal/search`), index that text, query with `OR` + bm25.
2. **nomic-embed-text is not usable for Chinese** (0.47; prefixes made it worse, 0.41).
   Default embedding is now **qwen3-embedding:0.6b** (1024d, ~0.2s/doc on CPU, fully local).
3. **RRF fusion is not worth it v0.1** once the vector side is good (0.94/0.87 < 0.94/0.92).
   Keyword search stays as the *fallback* path (cold start, no embedding service, model swap),
   not as a scoring boost.
4. **Hard timeout + degrade.** Query-side embedding gets a 3s budget; on timeout the search
   falls back to keyword only and reports `degraded=true`. (Competitor lesson: Basic Memory
   hangs for 60s when its embedding model cannot be downloaded.)

Reproduce:

```bash
go run ./cmd/exp-retrieval      # needs local Ollama with the two models pulled
```

Caveat: synthetic 60-doc corpus with visible lexical overlap, Chinese only — good enough to
reject the old defaults, not enough to tune fusion weights. Re-test at S14 with real sessions.
