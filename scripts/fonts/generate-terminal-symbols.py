#!/usr/bin/env python3
"""Write the small font that draws the media control symbols in the terminal.

Claude Code marks its modes and tool calls with U+23F4 to U+23FA. Android has no
text font with them: U+23F4 to U+23F7 come out as empty boxes and the rest as
colour emoji. The shapes are drawn here, one terminal cell wide, so that two in a
row (the mode mark) stand side by side. It needs fontTools with WOFF2 support:

    pip install fonttools brotli
    python3 scripts/fonts/generate-terminal-symbols.py
"""
from pathlib import Path

from fontTools.fontBuilder import FontBuilder
from fontTools.pens.ttGlyphPen import TTGlyphPen

ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "web/public/fonts/SshcTerminalSymbols.woff2"
FAMILY = "sshc Terminal Symbols"

UNITS_PER_EM = 1000
# The cell of the monospace fonts the terminal uses (JetBrains Mono, Roboto Mono, Droid Sans Mono).
CELL_WIDTH = 600
ASCENT = 900
DESCENT = -200
# The shapes are centred on the middle of a capital letter, where the text beside them is.
CENTRE_X = CELL_WIDTH // 2
CENTRE_Y = 340

# Half the size of each shape. A triangle is narrower along its direction, as a play button is.
TRIANGLE_LENGTH = 200
TRIANGLE_SPREAD = 280
SQUARE_HALF = 210
CIRCLE_RADIUS = 215
# tan(22.5 degrees): how far along the tangent the control points of an eight-part circle lie.
CIRCLE_CONTROL = 0.414
PAUSE_BAR_WIDTH = 120
PAUSE_BAR_GAP = 100
PAUSE_HALF_HEIGHT = 280
# A fixed time (seconds since 1904, as the font's head table counts), so the same shapes make the same file.
WRITTEN_AT = 3_877_000_000


def draw_polygon(pen, *points):
    pen.moveTo(points[0])
    for point in points[1:]:
        pen.lineTo(point)
    pen.closePath()


def draw_triangle(pen, dx, dy):
    """A triangle pointing along (dx, dy), one of the four directions."""
    tip = (CENTRE_X + dx * TRIANGLE_LENGTH, CENTRE_Y + dy * TRIANGLE_LENGTH)
    base_x, base_y = CENTRE_X - dx * TRIANGLE_LENGTH, CENTRE_Y - dy * TRIANGLE_LENGTH
    side_x, side_y = dy * TRIANGLE_SPREAD, dx * TRIANGLE_SPREAD
    draw_polygon(pen, (base_x + side_x, base_y + side_y), tip, (base_x - side_x, base_y - side_y))


def draw_rectangle(pen, left, bottom, right, top):
    draw_polygon(pen, (left, bottom), (left, top), (right, top), (right, bottom))


def draw_pause(pen):
    inner = PAUSE_BAR_GAP // 2
    outer = inner + PAUSE_BAR_WIDTH
    bottom = CENTRE_Y - PAUSE_HALF_HEIGHT
    top = CENTRE_Y + PAUSE_HALF_HEIGHT
    draw_rectangle(pen, CENTRE_X - outer, bottom, CENTRE_X - inner, top)
    draw_rectangle(pen, CENTRE_X + inner, bottom, CENTRE_X + outer, top)


def draw_square(pen):
    draw_rectangle(pen, CENTRE_X - SQUARE_HALF, CENTRE_Y - SQUARE_HALF, CENTRE_X + SQUARE_HALF, CENTRE_Y + SQUARE_HALF)


def draw_circle(pen):
    far = CIRCLE_RADIUS
    near = round(CIRCLE_RADIUS * CIRCLE_CONTROL)
    controls = [(far, -near), (near, -far), (-near, -far), (-far, -near), (-far, near), (-near, far), (near, far), (far, near)]
    # A contour of control points alone, closed by the None: the curve passes halfway between them.
    pen.qCurveTo(*[(CENTRE_X + x, CENTRE_Y + y) for x, y in controls], None)
    pen.closePath()


SHAPES = {
    0x23F4: lambda pen: draw_triangle(pen, -1, 0),
    0x23F5: lambda pen: draw_triangle(pen, 1, 0),
    0x23F6: lambda pen: draw_triangle(pen, 0, 1),
    0x23F7: lambda pen: draw_triangle(pen, 0, -1),
    0x23F8: draw_pause,
    0x23F9: draw_square,
    0x23FA: draw_circle,
}


def glyph_name(codepoint: int) -> str:
    return f"uni{codepoint:04X}"


def drawn_glyph(draw):
    pen = TTGlyphPen(None)
    draw(pen)
    return pen.glyph()


def main() -> None:
    glyphs = {".notdef": TTGlyphPen(None).glyph()}
    glyphs.update({glyph_name(codepoint): drawn_glyph(draw) for codepoint, draw in SHAPES.items()})

    builder = FontBuilder(UNITS_PER_EM, isTTF=True)
    builder.setupGlyphOrder(list(glyphs))
    builder.setupCharacterMap({codepoint: glyph_name(codepoint) for codepoint in SHAPES})
    builder.setupGlyf(glyphs)
    glyf = builder.font["glyf"]
    builder.setupHorizontalMetrics({name: (CELL_WIDTH, getattr(glyf[name], "xMin", 0)) for name in glyphs})
    builder.setupHorizontalHeader(ascent=ASCENT, descent=DESCENT)
    builder.setupNameTable({"familyName": FAMILY, "styleName": "Regular"})
    builder.setupOS2(sTypoAscender=ASCENT, sTypoDescender=DESCENT, usWinAscent=ASCENT, usWinDescent=-DESCENT)
    builder.setupPost(isFixedPitch=1)
    builder.updateHead(created=WRITTEN_AT, modified=WRITTEN_AT)
    builder.font.recalcTimestamp = False
    builder.font.flavor = "woff2"
    builder.save(OUTPUT)


if __name__ == "__main__":
    main()
