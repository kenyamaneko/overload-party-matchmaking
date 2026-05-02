#!/usr/bin/env python3
"""data/models.yaml から packages/api-matchmaking/*_gen.go を生成する.

共通基盤 `overload-party-codegen-tools` の低レベル API (`render_go_file`) を使う。
matchmaking は YAML スキーマが他リポと違い、セクション直下に `path` (リポ相対) と
`package` を持つため、`CodegenRunner` の標準パイプラインではなく独自の薄いループで
ファイル単位に組み立てる。
"""

from __future__ import annotations

import sys
from pathlib import Path

from codegen_tools import GoStyle, load_yaml, render_go_file

ROOT = Path(__file__).resolve().parent.parent
MODELS_YAML = ROOT / "data" / "models.yaml"

STYLE = GoStyle(
    use_inline_single_import=True,
    type_comment_key="doc",
    type_comment_multiline=True,
    field_comment_key="doc",
    field_comment_position="above",
    field_align=False,
    field_extra_tag_key="tag",
    tag_keys=("json",),
)


def main() -> int:
    data = load_yaml(MODELS_YAML)
    sections = data.get("files", [])
    if not sections:
        sys.stderr.write(f"error: {MODELS_YAML.name} has no `files:` section\n")
        return 1

    for section in sections:
        body = render_go_file(
            package=section["package"],
            types=section["types"],
            style=STYLE,
        )
        out = ROOT / section["path"]
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(body, encoding="utf-8")
        print(f"Generated → {section['path']}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
