# Semantic colors

Semantic mode colors each word by its contextual embedding, so similar embeddings get similar colors. Method, limits, code and costs.

## Overview

1. The embedding model produces a contextual vector for each word.
2. For each document, the server computes the two principal axes of that document's word vectors and projects every word onto the plane they span.
3. A word's angle in that plane sets its OKLCH hue, and its distance from the document mean sets its chroma. Lightness is constant.
4. On each edit the plane is refit from the previous one and aligned with it, so existing words keep their colors.

## Example

[semantic-example.txt](../examples/semantic-example.txt) is a single paragraph in which every sentence mixes words from four semantic groups:

> A fox found some bread near the old computer and seemed happy. Later a deer ate an apple beside the printer, looking scared. …

The groups are interleaved, so any color grouping comes from the words' meanings rather than their position in the text. With the default model and settings:

| group      | words                                                      | color              |
| ---------- | ---------------------------------------------------------- | ------------------ |
| animals    | fox, deer, owl, rabbit, wolf, bear                         | teal/cyan          |
| food       | bread, apple, cheese, honey, butter, soup                  | orange/salmon      |
| technology | computer, printer, keyboard, router, server, phone         | amber/gold         |
| emotions   | happy, scared, angry, calm, proud, lonely, tired, cheerful | pink/orchid        |

The mean color distance between words in different groups is 3.9 times the mean distance between words in the same group. Food and technology sit in neighboring hues (about 40-65° and 50-95°), so they are easier to tell apart by context than by color; animals and emotions are clearly separate.

Two known behaviors are excluded from the example:

- **Short standalone paragraphs.** Paragraphs are embedded independently. In a short paragraph, every word's embedding is dominated by the same passage-level component, so the paragraph tends to render as a single saturated color. The one-sentence final paragraph of [test.txt](../examples/test.txt) shows this; merging it into the preceding paragraph restores per-word variation.
- **Polysemy.** Different senses of the same word (for example, "bank" as riverside and as financial institution) receive nearly identical colors, because these models' embeddings are dominated by word identity rather than sense.

## 1. Word embeddings

The model (`paraphrase-multilingual-MiniLM-L12-v2`, the only semantic model in the catalog) processes one chunk of text at a time and returns a hidden state for each subword token. A word's vector is the mean of its subword vectors:

$$v_w = \frac{1}{|S(w)|} \sum_{s \in S(w)} h_s$$

Because the hidden states are contextual, the same word can receive different vectors in different sentences.

Vectors are normalized to unit length, $\hat v = v / \lVert v \rVert$. Similarity is measured by cosine distance, $1 - \hat v \cdot \hat w \in [0, 2]$, or equivalently by the chord length $\lVert \hat v - \hat w \rVert = \sqrt{2 - 2\,\hat v \cdot \hat w}$. The vectors have 384 dimensions. The method reduces them to two.

## 2. Per-document principal plane

Let $m = \frac{1}{n}\sum_i \hat v_i$ be the document mean. Let $b_0, b_1$ be the top two eigenvectors of the scatter matrix

$$C = \sum_i (\hat v_i - m)(\hat v_i - m)^\top.$$

Each word is projected onto the plane they span:

$$z_i = \big((\hat v_i - m) \cdot b_0,\ (\hat v_i - m) \cdot b_1\big)$$

Of all orthogonal projections to two dimensions, this one preserves the most variance: about 12-13% of the total for 1,000-5,000 words of news and web text. A fixed random plane preserves about 0.6%.

### Computation

$C$ is never materialized; at $d = 384$ it would occupy 590 KB. The method needs only products $Ca$, which can be computed directly from the data in a single pass:

$$C a = \sum_i (\hat v_i - m)\,\big((\hat v_i - m)\cdot a\big)$$

`covMul` evaluates this for both axes at once. The axes themselves are found by subspace iteration: multiply the current pair by $C$, re-orthonormalize with Gram–Schmidt, and repeat. Each iteration costs $O(nd)$.

## 3. Stability across edits

**Warm start.** Each refit starts from the document's previous axes and runs at most 3 iterations; a document's first fit runs up to 30. An edit changes the document only slightly, so three warm iterations land close to the converged result.

The cap matters because convergence slows when the second and third eigenvalues are close; without it, refits of long documents run to the iteration limit on every edit.

**Alignment.** PCA determines the plane but not the orientation of the axes within it. A refit may return the same plane rotated or reflected relative to the previous fit, which would shift every hue even if the document were unchanged. After each refit, the new axes $b_0, b_1$ are rotated or reflected to best match the previous axes $a_0, a_1$. With $M_{ij} = a_i \cdot b_j$:

- optimal rotation angle: $\theta = \mathrm{atan2}(M_{10} - M_{01},\ M_{00} + M_{11})$
- optimal reflection angle: $\varphi = \mathrm{atan2}(M_{01} + M_{10},\ M_{00} - M_{11})$

The candidate with the larger overlap $\sum_i a_i \cdot b'_i$ is applied (`alignPlane`). This is the closed-form solution of the 2D orthogonal Procrustes problem. Without alignment, most word colors would jump on a typical edit.

**Small documents.** Below 50 words the principal axes are too noisy to be useful. These documents use a fixed random plane, generated from a constant seed, which also serves as the starting point for a document's first fit.

## 4. Mapping to color

Colors are specified in OKLCH, a cylindrical form of the OKLab perceptual color space:

- **Hue** is the angle of the word's projected position.
- **Chroma** is proportional to its distance from the mean, capped at `semanticChroma`.
- **Lightness** is the same for all words (`semanticLightness`), so no word stands out by brightness alone.

Projected positions are scaled by a factor $s$ and clamped to the unit disk:

$$u = \mathrm{clamp}(s \cdot z), \qquad \text{chroma} = C_{\max} \cdot |u|, \qquad \text{hue} = \mathrm{atan2}(u_y, u_x)$$

Words on the boundary of the disk receive the full configured chroma. Words near the document mean receive less: about 1–2% of words in typical text are close to neutral gray (radius below 0.15).

### Disk mapping versus angle-only mapping

An angle-only mapping (all words at full chroma) is unstable near the origin. A position close to the mean can move to the opposite hue under an arbitrarily small perturbation, so near-identical embeddings can receive opposite colors.

The disk mapping is Lipschitz. Orthogonal projection does not increase distances, and clamping onto a convex set does not either, so

$$|u_i - u_j| \le s\,\lVert z_i - z_j\rVert \le s\,\lVert \hat v_i - \hat v_j\rVert.$$

Embeddings that are close always receive colors that are close.

### Scale selection

A small $s$ makes all colors gray; a large $s$ pushes all words to full chroma. $s$ is chosen so that normalized color distance matches normalized embedding distance:

- color distance: $|u_i - u_j| / 2$ (the disk has diameter 2)
- embedding distance: $\lVert \hat v_i - \hat v_j \rVert / 2$

`fitScale` samples 500 word pairs and evaluates 79 candidate scales, each a multiple of $1/r_{95}$, where $r_{95}$ is the 95th-percentile projected radius. It selects the candidate with the lowest mean absolute difference between the two distances.

The previous scale is kept unless the new best improves the sampled error by more than 2%. Without this hysteresis, sampling variation between edits would move $s$ between adjacent candidates and change the chroma of every word.

## 5. Conversion to sRGB

1. **Quantization.** Hue is rounded to 3° steps and chroma to 1/16 of the maximum. One step corresponds to an OKLab difference of at most about 0.009, below half of a just-noticeable difference (about 0.02). Quantization lets small refit drift map to the same hex value. This matters because the VS Code extension creates one decoration type per distinct hex color. It also bounds the palette to about 1,900 colors.
2. **OKLCH → OKLab:** $a = \text{chroma}\cdot\cos h$, $b = \text{chroma}\cdot\sin h$.
3. **OKLab → linear sRGB:** Björn Ottosson's published OKLab matrices, with a cube between them.
4. **Encoding:** the sRGB transfer function, a clamp to 0–255, and `#RRGGBB` formatting.

At lightness 0.75, a chroma of 0.14 lies outside the sRGB gamut for hues between about 177° and 289°, and those colors are clipped per channel. A `semanticChroma` of 0.127 or less keeps every hue in gamut.

## 6. Accuracy and limits

The table below was measured on the first 1,000 and 5,000 words of the UD English-EWT test set, over all word pairs. "Mean error" is the mean absolute difference between normalized color distance and normalized embedding distance.

| method                               | Pearson correlation | mean error |
| ------------------------------------ | ------------------- | ---------- |
| fixed random plane, angle only       | 0.19                | 0.27       |
| per-document PCA, angle only         | 0.46                | 0.25       |
| per-document PCA, disk (shipped)     | 0.46–0.47           | 0.23       |

Three limits apply to any linear projection to two dimensions, independent of tuning:

- **Distinct embeddings can share a color.** The projection discards $d - 2$ dimensions. Two embeddings that differ only in discarded directions map to the same point, including in the extreme case of opposite vectors.
- **Mean error has a nonzero floor.** Embedding distances in natural text are concentrated in a narrow range, while distances between points spread over a plane are not. The difference between the two distributions sets a lower bound of about 0.16–0.25 on the mean error. The disk mapping lowers this floor relative to the angle-only mapping, because positions inside the disk allow color distances to concentrate as well.
- **Close embeddings yield close colors** (section 4).

Shared or similar colors therefore indicate likely, not certain, similarity in usage.

## 7. Implementation

Pooling: `embedChunk` in [`embedding_model.go`](../../internal/inference/embedding_model.go). Fit, alignment, scale and color: `SemanticPlane.Fit` and `SemanticPlane.Color` in [`semantic_color.go`](../../internal/inference/semantic_color.go) (`covMul`, `alignPlane`, `fitScale`, `diskColor`, `oklchToSRGB`; fallback plane `InitSemantic`). Per-document state and recoloring: `documentStore.plane` and `docTokens` in [`registry.go`](../../internal/lspserver/registry.go). VS Code rendering: `handleSemanticColors` in [`extension.js`](../../vscode-extension/extension.js).

Processing of one edit:

1. The registry splits the text into chunks and reuses the results of chunks whose words are unchanged, including their vectors.
2. Changed chunks are embedded by the model.
3. As each chunk completes, `docTokens` recolors the document using the current plane, and the server sends `$/nls/semanticColors`.
4. When all chunks have completed, `docTokens` refits the plane, recolors, and sends the final colors.

## 8. Memory and performance

**Retained per open document:**

| item                         | size                                                                                       |
| ---------------------------- | ------------------------------------------------------------------------------------------ |
| unit vector per word          | 4 bytes × 384 + 24-byte slice header: ≈1.5 KB/word |
| plane state (`SemanticPlane`) | mean and two axes, 3 × 4 × 384: 4.6 KB             |
| 5,000-word document           | ≈7.8 MB of vectors                                 |

Vectors are retained so that an edit re-embeds only changed chunks while the whole document is recolored. They are released when the document is closed.

**Per edit** (real embeddings of English-EWT text, ThinkPad X280):

| document    | first fit | warm refit (`Fit`) | coloring (`Color`, all words) |
| ----------- | --------- | ------------------ | ----------------------------- |
| 1,000 words | 32–37 ms  | 4 ms               | 1.3 ms                        |
| 5,000 words | 92–98 ms  | 14–15 ms           | 7 ms                          |

`Fit` makes 9 allocations: the mean, two axis pairs, three per-word arrays for scale selection and the pair sample. `Color` allocates only the hex string; `docTokens` adds the merged token slice and 24 bytes of vector reference per word.

For comparison, embedding a single 122-word chunk takes about 77 ms, so the model dominates edit time.

**Client side:**

- **VS Code:** the extension creates one decoration type per distinct hex color. Quantization bounds the number of types, and the extension disposes types that no open document uses.
- **Neovim:** the handler in [neovim.md](../neovim.md) creates one highlight group per distinct color, which quantization also bounds.

## 9. Behavioral notes

- The same word can get different colors in different documents, or after reopening one.
- Crossing 50 words in either direction changes all colors once; large insertions converge over several edits.
- Refits run on the registry goroutine, so hover and token requests wait: a few ms typically, about 20 ms for 5,000 words.
- Low-chroma words near the mean may look uncolored.
