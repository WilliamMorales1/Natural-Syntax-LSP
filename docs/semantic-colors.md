# Semantic colors

Semantic mode colors each word by its contextual embedding, so similar embeddings get similar colors. Method, limits, code and costs.

## Overview

1. The embedding model produces a contextual vector for each word.
2. For each document, the server computes the two principal axes of that document's word vectors and projects every word onto the plane they span.
3. A word's angle in that plane sets its OKLCH hue, and its distance from the document mean sets its chroma. Lightness is constant.
4. On each edit the plane is refit from the previous one and aligned with it, so existing words keep their colors.

## Example

[semantic-example.txt](examples/semantic-example.txt) is a single paragraph in which every sentence mixes words from four semantic groups:

> A fox found some bread near the old computer and seemed happy. Later a deer ate an apple beside the printer, looking scared. …

The groups are interleaved, so any color grouping comes from the words' meanings rather than their position in the text. With `all-mpnet-base-v2` and the default settings (measured before the default semantic model became `paraphrase-multilingual-MiniLM-L12-v2`):

| group      | words                                                    | color               |
| ---------- | -------------------------------------------------------- | ------------------- |
| animals    | fox, deer, owl, rabbit, wolf, bear                       | lavender/periwinkle |
| food       | bread, apple, cheese, honey, butter, soup                | sky blue            |
| technology | computer, printer, keyboard, router, server, phone       | green-teal          |
| emotions   | happy, scared, angry, calm, proud, lonely, tired, cheerful | muted pink-gray   |

The mean color distance between words in different groups is 3.6 times the mean distance between words in the same group. `all-MiniLM-L6-v2` also keeps each group compact, but places animals, food and technology in adjacent cyan hues; only the emotion words are clearly separated.

Two known behaviors are excluded from the example:

- **Short standalone paragraphs.** Paragraphs are embedded independently. In a short paragraph, every word's embedding is dominated by the same passage-level component, so the paragraph tends to render as a single saturated color. The one-sentence final paragraph of [test.txt](examples/test.txt) shows this; merging it into the preceding paragraph restores per-word variation.
- **Polysemy.** Different senses of the same word (for example, "bank" as riverside and as financial institution) receive nearly identical colors, because these models' embeddings are dominated by word identity rather than sense.

## 1. Word embeddings

The model (default `paraphrase-multilingual-MiniLM-L12-v2`; the numbers in this document were measured with `all-mpnet-base-v2` and `all-MiniLM-L6-v2`, which have been removed) processes one chunk of text at a time and returns a hidden state for each subword token. A word's vector is the mean of its subword vectors:

$$v_w = \frac{1}{|S(w)|} \sum_{s \in S(w)} h_s$$

Because the hidden states are contextual, the same word can receive different vectors in different sentences.

Vectors are normalized to unit length, $\hat v = v / \lVert v \rVert$. Similarity is measured by cosine distance, $1 - \hat v \cdot \hat w \in [0, 2]$, or equivalently by the chord length $\lVert \hat v - \hat w \rVert = \sqrt{2 - 2\,\hat v \cdot \hat w}$. The vectors have 384 dimensions (multilingual MiniLM; 768 for mpnet). The method reduces them to two.

## 2. Per-document principal plane

Let $m = \frac{1}{n}\sum_i \hat v_i$ be the document mean. Let $b_0, b_1$ be the top two eigenvectors of the scatter matrix

$$C = \sum_i (\hat v_i - m)(\hat v_i - m)^\top.$$

Each word is projected onto the plane they span:

$$z_i = \big((\hat v_i - m) \cdot b_0,\ (\hat v_i - m) \cdot b_1\big)$$

Of all orthogonal projections to two dimensions, this one preserves the most variance: about 22% of the total for a typical document. A fixed random plane, which earlier versions used, preserves about 0.3%.

### Computation

$C$ is never materialized; at $d = 768$ it would occupy 2.4 MB. The method needs only products $Ca$, which can be computed directly from the data in a single pass:

$$C a = \sum_i (\hat v_i - m)\,\big((\hat v_i - m)\cdot a\big)$$

`covMul` evaluates this for both axes at once. The axes themselves are found by subspace iteration: multiply the current pair by $C$, re-orthonormalize with Gram–Schmidt, and repeat. Each iteration costs $O(nd)$.

## 3. Stability across edits

**Warm start.** Each refit starts from the document's previous axes and runs at most 3 iterations; a document's first fit runs up to 30. Three warm iterations are within about 0.006 (in units of the maximum color difference) of the converged result.

The cap is required because convergence slows when the second and third eigenvalues are close. Without it, refits of a 5,000-word document ran to the iteration limit, taking about 290 ms per edit instead of about 50 ms.

**Alignment.** PCA determines the plane but not the orientation of the axes within it. A refit may return the same plane rotated or reflected relative to the previous fit, which would shift every hue even if the document were unchanged. After each refit, the new axes $b_0, b_1$ are rotated or reflected to best match the previous axes $a_0, a_1$. With $M_{ij} = a_i \cdot b_j$:

- optimal rotation angle: $\theta = \mathrm{atan2}(M_{10} - M_{01},\ M_{00} + M_{11})$
- optimal reflection angle: $\varphi = \mathrm{atan2}(M_{01} + M_{10},\ M_{00} - M_{11})$

The candidate with the larger overlap $\sum_i a_i \cdot b'_i$ is applied (`alignPlane`). This is the closed-form solution of the 2D orthogonal Procrustes problem. Without alignment, 67–81% of word colors shifted by more than a quarter of the maximum color difference on each edit.

**Small documents.** Below 50 words the principal axes are too noisy to be useful. These documents use a fixed random plane, generated from a constant seed, which also serves as the starting point for a document's first fit.

## 4. Mapping to color

Colors are specified in OKLCH, a cylindrical form of the OKLab perceptual color space:

- **Hue** is the angle of the word's projected position.
- **Chroma** is proportional to its distance from the mean, capped at `semanticChroma`.
- **Lightness** is the same for all words (`semanticLightness`), so no word stands out by brightness alone.

Projected positions are scaled by a factor $s$ and clamped to the unit disk:

$$u = \mathrm{clamp}(s \cdot z), \qquad \text{chroma} = C_{\max} \cdot |u|, \qquad \text{hue} = \mathrm{atan2}(u_y, u_x)$$

Words on the boundary of the disk receive the full configured chroma. Words near the document mean receive less: 1–8% of words in typical text are close to neutral gray.

### Disk mapping versus angle-only mapping

An angle-only mapping (all words at full chroma) is unstable near the origin. A position close to the mean can move to the opposite hue under an arbitrarily small perturbation, so near-identical embeddings can receive opposite colors.

The disk mapping is Lipschitz. Orthogonal projection does not increase distances, and clamping onto a convex set does not either, so

$$|u_i - u_j| \le s\,\lVert z_i - z_j\rVert \le s\,\lVert \hat v_i - \hat v_j\rVert.$$

Embeddings that are close always receive colors that are close.

### Scale selection

A small $s$ makes all colors gray; a large $s$ pushes all words to full chroma. $s$ is chosen so that normalized color distance matches normalized embedding distance:

- color distance: $|u_i - u_j| / 2$ (the disk has diameter 2)
- embedding distance: $\lVert \hat v_i - \hat v_j \rVert / 2$

`fitScale` samples 500 word pairs and evaluates 79 candidate scales, each a multiple of $1/r_{95}$, where $r_{95}$ is the 95th-percentile projected radius. It selects the candidate with the lowest mean absolute difference between the two distances. In testing, 500 pairs gave the same result as 20,000 to within 0.001.

The previous scale is kept unless the new best improves the sampled error by more than 2%. Without this hysteresis, sampling variation between edits moved $s$ between adjacent candidates and changed the chroma of every word.

## 5. Conversion to sRGB

1. **Quantization.** Hue is rounded to 3° steps and chroma to 1/16 of the maximum. One step corresponds to an OKLab difference of at most about 0.009, below half of a just-noticeable difference (about 0.02). Quantization lets small refit drift map to the same hex value. This matters because the VS Code extension creates one decoration type per distinct hex color. It also bounds the palette to about 1,900 colors.
2. **OKLCH → OKLab:** $a = \text{chroma}\cdot\cos h$, $b = \text{chroma}\cdot\sin h$.
3. **OKLab → linear sRGB:** Björn Ottosson's published OKLab matrices, with a cube between them.
4. **Encoding:** the sRGB transfer function, a clamp to 0–255, and `#RRGGBB` formatting.

At lightness 0.75, a chroma of 0.14 lies outside the sRGB gamut for hues between about 177° and 289°, and those colors are clipped per channel. A `semanticChroma` of 0.127 or less keeps every hue in gamut.

## 6. Accuracy and limits

The table below was measured on an excerpt of _Pride and Prejudice_ (1,000 and 5,000 words), over all word pairs. "Mean error" is the mean absolute difference between normalized color distance and normalized embedding distance.

| method                                  | Pearson correlation | mean error |
| --------------------------------------- | ------------------- | ---------- |
| fixed random plane, angle only (former) | 0.15–0.20           | 0.26–0.28  |
| per-document PCA, angle only            | 0.36–0.58           | 0.25–0.28  |
| per-document PCA, disk (current)        | 0.36–0.53           | 0.19–0.22  |

Three limits apply to any linear projection to two dimensions, independent of tuning:

- **Distinct embeddings can share a color.** The projection discards $d - 2$ dimensions. Two embeddings that differ only in discarded directions map to the same point, including in the extreme case of opposite vectors.
- **Mean error has a nonzero floor.** Embedding distances in natural text are concentrated in a narrow range, while distances between points spread over a plane are not. The difference between the two distributions sets a lower bound of about 0.16–0.25 on the mean error. The disk mapping lowers this floor relative to the angle-only mapping, because positions inside the disk allow color distances to concentrate as well.
- **Close embeddings yield close colors** (section 4).

Shared or similar colors therefore indicate likely, not certain, similarity in usage.

## 7. Implementation

Pooling: `embedChunk` in [`embedding_model.go`](../internal/inference/embedding_model.go). Fit, alignment, scale and color: `SemanticPlane.Fit` and `SemanticPlane.Color` in [`semantic_color.go`](../internal/inference/semantic_color.go) (`covMul`, `alignPlane`, `fitScale`, `diskColor`, `oklchToSRGB`; fallback plane `InitSemantic`). Per-document state and recoloring: `documentStore.plane` and `docTokens` in [`registry.go`](../internal/lspserver/registry.go). VS Code rendering: `handleSemanticColors` in [`extension.js`](../vscode-extension/extension.js).

Processing of one edit:

1. The registry splits the text into chunks and reuses the results of chunks whose words are unchanged, including their vectors.
2. Changed chunks are embedded by the model.
3. As each chunk completes, `docTokens` recolors the document using the current plane, and the server sends `$/nls/semanticColors`.
4. When all chunks have completed, `docTokens` refits the plane, recolors, and sends the final colors.

## 8. Memory and performance

**Retained per open document:**

| item                         | size                                                                                       |
| ---------------------------- | ------------------------------------------------------------------------------------------ |
| unit vector per word         | 4 bytes × dimension + 24-byte slice header: ≈1.5 KB/word (MiniLM), ≈3.1 KB/word (mpnet)    |
| plane state (`SemanticPlane`) | mean and two axes, 3 × 4 × dimension: 4.6 KB (MiniLM), 9.2 KB (mpnet)                     |
| 5,000-word document, mpnet   | ≈15.5 MB of vectors                                                                        |

Vectors are retained so that an edit re-embeds only changed chunks while the whole document is recolored. They are released when the document is closed.

**Per edit** (Go benchmarks on real embeddings, with allocation counts):

| document            | refit (`Fit`)                   | coloring (`Color`, all words)  |
| ------------------- | ------------------------------- | ------------------------------ |
| MiniLM, 1,000 words | 5.2 ms, 25 KB in 9 allocations  | 1.3 ms, 1 allocation per word  |
| mpnet, 1,000 words  | 12.1 ms, 33 KB in 9 allocations | 2.2 ms, 1 allocation per word  |
| mpnet, 5,000 words  | 51 ms, 82 KB in 9 allocations   | 11.5 ms, 1 allocation per word |

`Fit`'s 9 allocations are the mean, two axis pairs, three per-word arrays for scale selection and the pair sample. `Color` allocates only the hex string; `docTokens` adds the merged token slice and 24 bytes of vector reference per word.

A document's first fit runs up to 30 iterations: 74 ms for 1,000 mpnet words and 415 ms for 5,000. Subsequent edits use the warm costs above.

For comparison, embedding a single 122-word chunk takes about 19 ms (MiniLM) or 104 ms (mpnet).

**Client side:**

- **VS Code:** the extension creates one decoration type per distinct hex color. Quantization bounds the number of types, and the extension disposes types that no open document uses.
- **Neovim:** the handler in [neovim.md](neovim.md) creates one highlight group per distinct color, which quantization also bounds.

## 9. Behavioral notes

- The same word can get different colors in different documents, or after reopening one.
- Crossing 50 words in either direction changes all colors once; large insertions converge over several edits.
- Refits run on the registry goroutine, so hover and token requests wait: a few ms typically, about 60 ms for 5,000 mpnet words.
- Low-chroma words near the mean may look uncolored.
