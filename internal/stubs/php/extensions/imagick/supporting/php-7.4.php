<?php

// Signatures adapted from the extension maintainers' own Zend API declarations.
// See internal/stubs/php/extensions/README.md for sources, API-version policy, and licenses.
/** ImagickDraw extension API. */
class ImagickDraw {
    /** Reset vector graphics. */ public function resetVectorGraphics(): bool {}
    /** Get text kerning. */ public function getTextKerning(): float {}
    /** Set text kerning. */ public function setTextKerning(float $kerning): bool {}
    /** Get text interword spacing. */ public function getTextInterwordSpacing(): float {}
    /** Set text interword spacing. */ public function setTextInterwordSpacing(float $spacing): bool {}
    /** Get text interline spacing. */ public function getTextInterlineSpacing(): float {}
    /** Set text interline spacing. */ public function setTextInterlineSpacing(float $spacing): bool {}
    /** Create an instance. */ public function __construct() {}
    /** Set fill color. */ public function setFillColor(ImagickPixel|string $fill_color): bool {}
    /** Set fill alpha. */ public function setFillAlpha(float $alpha): bool {}
    /** Set resolution. */ public function setResolution(float $resolution_x, float $resolution_y): bool {}
    /** Set stroke color. */ public function setStrokeColor(ImagickPixel|string $color): bool {}
    /** Set stroke alpha. */ public function setStrokeAlpha(float $alpha): bool {}
    /** Set stroke width. */ public function setStrokeWidth(float $width): bool {}
    /** Clear. */ public function clear(): bool {}
    /** Circle. */ public function circle( float $origin_x, float $origin_y, float $perimeter_x, float $perimeter_y ): bool {}
    /** Annotation. */ public function annotation(float $x, float $y, string $text): bool {}
    /** Set text antialias. */ public function setTextAntialias(bool $antialias): bool {}
    /** Set text encoding. */ public function setTextEncoding(string $encoding): bool {}
    /** Set font. */ public function setFont(string $font_name): bool {}
    /** Set font family. */ public function setFontFamily(string $font_family): bool {}
    /** Set font size. */ public function setFontSize(float $point_size): bool {}
    /** Set font style. */ public function setFontStyle(int $style): bool {}
    /** Set font weight. */ public function setFontWeight(int $weight): bool {}
    /** Get font. */ public function getFont(): string {}
    /** Get font family. */ public function getFontFamily(): string {}
    /** Get font size. */ public function getFontSize(): float {}
    /** Get font style. */ public function getFontStyle(): int {}
    /** Get font weight. */ public function getFontWeight(): int {}
    /** Destroy. */ public function destroy(): bool {}
    /** Rectangle. */ public function rectangle(float $top_left_x, float $top_left_y, float $bottom_right_x, float $bottom_right_y): bool {}
    /** Round rectangle. */ public function roundRectangle(float $top_left_x, float $top_left_y, float $bottom_right_x, float $bottom_right_y, float $rounding_x, float $rounding_y): bool {}
    /** Ellipse. */ public function ellipse(float $origin_x, float $origin_y, float $radius_x, float $radius_y, float $angle_start, float $angle_end): bool {}
    /** Skew x. */ public function skewX(float $degrees): bool {}
    /** Skew y. */ public function skewY(float $degrees): bool {}
    /** Translate. */ public function translate(float $x, float $y): bool {}
    /** Line. */ public function line(float $start_x, float $start_y, float $end_x, float $end_y): bool {}
    /** Arc. */ public function arc( float $start_x, float $start_y, float $end_x, float $end_y, float $start_angle, float $end_angle ): bool {}
    /** Alpha. */ public function alpha(float $x, float $y, int $paint): bool {}
    /** Matte. */ public function matte(float $x, float $y, int $paint): bool {}
    /** Polygon. */ public function polygon(array $coordinates): bool {}
    /** Point. */ public function point(float $x, float $y): bool {}
    /** Get text decoration. */ public function getTextDecoration(): int {}
    /** Get text encoding. */ public function getTextEncoding(): false|string {}
    /** Get font stretch. */ public function getFontStretch(): int {}
    /** Set font stretch. */ public function setFontStretch(int $stretch): bool {}
    /** Set stroke antialias. */ public function setStrokeAntialias(bool $enabled): bool {}
    /** Set text alignment. */ public function setTextAlignment(int $align): bool {}
    /** Set text decoration. */ public function setTextDecoration(int $decoration): bool {}
    /** Set text under color. */ public function setTextUnderColor(ImagickPixel|string $under_color): bool {}
    /** Set viewbox. */ public function setViewbox(int $left_x, int $top_y, int $right_x, int $bottom_y): bool {}
    /** Clone. */ public function clone(): ImagickDraw {}
    /** Affine. */ public function affine(array $affine): bool {}
    /** Bezier. */ public function bezier(array $coordinates): bool {}
    /** Composite. */ public function composite( int $composite, float $x, float $y, float $width, float $height, Imagick $image ): bool {}
    /** Color. */ public function color(float $x, float $y, int $paint): bool {}
    /** Comment. */ public function comment(string $comment): bool {}
    /** Get clip path. */ public function getClipPath(): false|string {}
    /** Get clip rule. */ public function getClipRule(): int {}
    /** Get clip units. */ public function getClipUnits(): int {}
    /** Get fill color. */ public function getFillColor(): ImagickPixel {}
    /** Get fill opacity. */ public function getFillOpacity(): float {}
    /** Get fill rule. */ public function getFillRule(): int {}
    /** Get gravity. */ public function getGravity(): int {}
    /** Get stroke antialias. */ public function getStrokeAntialias(): bool {}
    /** Get stroke color. */ public function getStrokeColor(): ImagickPixel {}
    /** Get stroke dash array. */ public function getStrokeDashArray(): array {}
    /** Get stroke dash offset. */ public function getStrokeDashOffset(): float {}
    /** Get stroke line cap. */ public function getStrokeLineCap(): int {}
    /** Get stroke line join. */ public function getStrokeLineJoin(): int {}
    /** Get stroke miter limit. */ public function getStrokeMiterLimit(): int {}
    /** Get stroke opacity. */ public function getStrokeOpacity(): float {}
    /** Get stroke width. */ public function getStrokeWidth(): float {}
    /** Get text alignment. */ public function getTextAlignment(): int {}
    /** Get text antialias. */ public function getTextAntialias(): bool {}
    /** Get vector graphics. */ public function getVectorGraphics(): string {}
    /** Get text under color. */ public function getTextUnderColor(): ImagickPixel {}
    /** Path close. */ public function pathClose(): bool {}
    /** Path curve to absolute. */ public function pathCurveToAbsolute(float $x1, float $y1, float $x2, float $y2, float $x, float $y): bool {}
    /** Path curve to relative. */ public function pathCurveToRelative(float $x1, float $y1, float $x2, float $y2, float $x, float $y): bool {}
    /** Path curve to quadratic bezier absolute. */ public function pathCurveToQuadraticBezierAbsolute(float $x1, float $y1, float $x_end, float $y): bool {}
    /** Path curve to quadratic bezier relative. */ public function pathCurveToQuadraticBezierRelative(float $x1, float $y1, float $x_end, float $y): bool {}
    /** Path curve to quadratic bezier smooth absolute. */ public function pathCurveToQuadraticBezierSmoothAbsolute(float $x, float $y): bool {}
    /** Path curve to quadratic bezier smooth relative. */ public function pathCurveToQuadraticBezierSmoothRelative(float $x, float $y): bool {}
    /** Path curve to smooth absolute. */ public function pathCurveToSmoothAbsolute(float $x2, float $y2, float $x, float $y): bool {}
    /** Path curve to smooth relative. */ public function pathCurveToSmoothRelative(float $x2, float $y2, float $x, float $y): bool {}
    /** Path elliptic arc absolute. */ public function pathEllipticArcAbsolute( float $rx, float $ry, float $x_axis_rotation, bool $large_arc, bool $sweep, float $x, float $y ): bool {}
    /** Path elliptic arc relative. */ public function pathEllipticArcRelative( float $rx, float $ry, float $x_axis_rotation, bool $large_arc, bool $sweep, float $x, float $y ): bool {}
    /** Path finish. */ public function pathFinish(): bool {}
    /** Path line to absolute. */ public function pathLineToAbsolute(float $x, float $y): bool {}
    /** Path line to relative. */ public function pathLineToRelative(float $x, float $y): bool {}
    /** Path line to horizontal absolute. */ public function pathLineToHorizontalAbsolute(float $x): bool {}
    /** Path line to horizontal relative. */ public function pathLineToHorizontalRelative(float $x): bool {}
    /** Path line to vertical absolute. */ public function pathLineToVerticalAbsolute(float $y): bool {}
    /** Path line to vertical relative. */ public function pathLineToVerticalRelative(float $y): bool {}
    /** Path move to absolute. */ public function pathMoveToAbsolute(float $x, float $y): bool {}
    /** Path move to relative. */ public function pathMoveToRelative(float $x, float $y): bool {}
    /** Path start. */ public function pathStart(): bool {}
    /** Polyline. */ public function polyline(array $coordinates): bool {}
    /** Pop clip path. */ public function popClipPath(): bool {}
    /** Pop defs. */ public function popDefs(): bool {}
    /** Pop pattern. */ public function popPattern(): bool {}
    /** Push clip path. */ public function pushClipPath(string $clip_mask_id): bool {}
    /** Push defs. */ public function pushDefs(): bool {}
    /** Push pattern. */ public function pushPattern( string $pattern_id, float $x, float $y, float $width, float $height ): bool {}
    /** Render. */ public function render(): bool {}
    /** Rotate. */ public function rotate(float $degrees): bool {}
    /** Scale. */ public function scale(float $x, float $y): bool {}
    /** Set clip path. */ public function setClipPath(string $clip_mask): bool {}
    /** Set clip rule. */ public function setClipRule(int $fillrule): bool {}
    /** Set clip units. */ public function setClipUnits(int $pathunits): bool {}
    /** Set fill opacity. */ public function setFillOpacity(float $opacity): bool {}
    /** Set fill pattern url. */ public function setFillPatternUrl(string $fill_url): bool {}
    /** Set fill rule. */ public function setFillRule(int $fillrule): bool {}
    /** Set gravity. */ public function setGravity(int $gravity): bool {}
    /** Set stroke pattern url. */ public function setStrokePatternUrl(string $stroke_url): bool {}
    /** Set stroke dash offset. */ public function setStrokeDashOffset(float $dash_offset): bool {}
    /** Set stroke line cap. */ public function setStrokeLineCap(int $linecap): bool {}
    /** Set stroke line join. */ public function setStrokeLineJoin(int $linejoin): bool {}
    /** Set stroke miter limit. */ public function setStrokeMiterLimit(int $miterlimit): bool {}
    /** Set stroke opacity. */ public function setStrokeOpacity(float $opacity): bool {}
    /** Set vector graphics. */ public function setVectorGraphics(string $xml): bool {}
    /** Pop. */ public function pop(): bool {}
    /** Push. */ public function push(): bool {}
    /** Set stroke dash array. */ public function setStrokeDashArray(array|null $dashes): bool {}
    /** Get opacity. */ public function getOpacity(): float {}
    /** Set opacity. */ public function setOpacity(float $opacity): bool {}
    /** Get font resolution. */ public function getFontResolution(): array {}
    /** Set font resolution. */ public function setFontResolution(float $x, float $y): bool {}
    /** Get border color. */ public function getBorderColor(): ImagickPixel {}
    /** Set border color. */ public function setBorderColor(ImagickPixel|string $color): bool {}
    /** Set density. */ public function setDensity(string $density): bool {}
    /** Get density. */ public function getDensity(): ?string {}
    /** Get text direction. */ public function getTextDirection(): int {}
    /** Set text direction. */ public function setTextDirection(int $direction): bool {}
}

/** ImagickPixel extension API. */
class ImagickPixel {
    /** Create an instance. */ public function __construct(?string $color = null) {}
    /** Clear. */ public function clear(): bool {}
    /** Destroy. */ public function destroy(): bool {}
    /** Get color. */ public function getColor(int $normalized = 0): array {}
    /** Get color as string. */ public function getColorAsString(): string {}
    /** Get color count. */ public function getColorCount(): int {}
    /** Get color quantum. */ public function getColorQuantum(): array {}
    /** Get color value. */ public function getColorValue(int $color): float {}
    /** Get color value quantum. */ public function getColorValueQuantum(int $color): int|float {}
    /** Get hsl. */ public function getHSL(): array {}
    /** Get index. */ public function getIndex(): int {}
    /** Is pixel similar. */ public function isPixelSimilar(ImagickPixel|string $color, float $fuzz): ?bool {}
    /** Is pixel similar quantum. */ public function isPixelSimilarQuantum(ImagickPixel|string $color, float $fuzz_quantum_range_scaled_by_square_root_of_three): ?bool {}
    /** Is similar. */ public function isSimilar( ImagickPixel|string $color, float $fuzz_quantum_range_scaled_by_square_root_of_three ): ?bool {}
    /** Set color. */ public function setColor(string $color): bool {}
    /** Set color count. */ public function setColorCount(int $color_count): bool {}
    /** Set color value. */ public function setColorValue(int $color, float $value): bool {}
    /** Set color value quantum. */ public function setColorValueQuantum(int $color, int|float $value): bool {}
    /** Set hsl. */ public function setHSL(float $hue, float $saturation , float $luminosity): bool {}
    /** Set index. */ public function setIndex(int|float $index): bool {}
    /** Set color from pixel. */ public function setColorFromPixel(ImagickPixel $pixel): bool {}
}

/** ImagickPixelIterator extension API. */
class ImagickPixelIterator implements Iterator {
    /** Create an instance. */ public function __construct(Imagick $imagick) {}
    /** Clear. */ public function clear(): bool {}
    /** Get pixel iterator. */ public static function getPixelIterator(Imagick $imagick) : ImagickPixelIterator {}
    /** Get pixel region iterator. */ public static function getPixelRegionIterator( Imagick $imagick, int $x, int $y, int $columns, int $rows): ImagickPixelIterator {}
    /** Destroy. */ public function destroy(): bool {}
    /** Get current iterator row. */ public function getCurrentIteratorRow(): ?array {}
    /** Get iterator row. */ public function getIteratorRow(): int {}
    /** Get next iterator row. */ public function getNextIteratorRow(): ?array {}
    /** Get previous iterator row. */ public function getPreviousIteratorRow(): array {}
    /** Key. */ public function key(): int {}
    /** Next. */ public function next(): void {}
    /** Rewind. */ public function rewind(): void {}
    /** Current. */ public function current(): ?array {}
    /** New pixel iterator. */ public function newPixelIterator(Imagick $imagick): bool {}
    /** New pixel region iterator. */ public function newPixelRegionIterator( Imagick $imagick, int $x, int $y, int $columns, int $rows): bool {}
    /** Reset iterator. */ public function resetIterator(): bool {}
    /** Set iterator first row. */ public function setIteratorFirstRow(): bool {}
    /** Set iterator last row. */ public function setIteratorLastRow(): bool {}
    /** Set iterator row. */ public function setIteratorRow(int $row): bool {}
    /** Sync iterator. */ public function syncIterator(): bool {}
    /** Valid. */ public function valid(): bool {}
}

/** ImagickKernel extension API. */
class ImagickKernel {
    /** Add kernel. */ public function addKernel(ImagickKernel $kernel): void {}
    /** Add unity kernel. */ public function addUnityKernel(float $scale): void {}
    /** From builtin. */ public static function fromBuiltin(int $kernel, string $shape): ImagickKernel {}
    /** From matrix. */ public static function fromMatrix(array $matrix, ?array $origin = null): ImagickKernel {}
    /** Get matrix. */ public function getMatrix(): array {}
    /** Scale. */ public function scale(float $scale, ?int $normalize_kernel = null): void {}
    /** Separate. */ public function separate(): array {}
}
