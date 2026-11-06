# Data

| Folder | What it holds |
|---|---|
| `sources/` | Original pages as downloaded. Never edited by hand. |
| `raw/` | Clean `.txt` documents that `ragscope ingest` reads. Generated from `sources/`. |
| `golden/` | The golden set used by `ragscope eval`. |

## LGPD (Lei nº 13.709/2018)

- **Source:** `sources/L13709.html`, the compiled text from
  <https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2018/lei/l13709.htm>,
  saved on 2026-11-06 (windows-1252, as published).
- **Why not copy and paste the text:** the page shows revoked wording struck
  through. Plain-text copy loses the strike, and old and current versions get
  mixed (the same inciso appeared up to 5 times). The converter reads the HTML
  and drops the struck parts.
- **Regenerate `raw/lgpd.txt`:**

  ```bash
  make corpus
  ```

- **Known noise left in the text:** the link text "Vigência" and the amendment
  notes such as "(Redação dada pela Lei nº 13.853, de 2019)". The source also
  has a typo, `Art. 5 7. (VETADO)`, which is the revoked art. 57.

Official legal texts are not protected by copyright (Lei nº 9.610/1998, art. 8º, IV).

To update the law, save the page again over `sources/L13709.html`, run
`make corpus`, then run `ragscope golden` to check the golden set still matches
the text.
