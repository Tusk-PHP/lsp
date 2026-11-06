<?php

// Signatures adapted from the extension maintainers' own Zend API declarations.
// See internal/stubs/php/extensions/README.md for sources, API-version policy, and licenses.
/** Imagick extension API. */
class Imagick implements Iterator, Countable {
    /** Optimize image layers. */ public function optimizeImageLayers(): Imagick {}
    /** Compare image layers. */ public function compareImageLayers(int $metric): Imagick {}
    /** Ping image blob. */ public function pingImageBlob(string $image): bool {}
    /** Ping image file. */ public function pingImageFile(mixed $filehandle, ?string $filename = null): bool {}
    /** Transpose image. */ public function transposeImage(): bool {}
    /** Transverse image. */ public function transverseImage(): bool {}
    /** Trim image. */ public function trimImage(float $fuzz): bool {}
    /** Wave image. */ public function waveImage(float $amplitude, float $length): bool {}
    /** Wave image with method. */ public function waveImageWithMethod( float $amplitude, float $length, int $interpolate_method ): bool {}
    /** Vignette image. */ public function vignetteImage(float $black_point, float $white_point, int $x, int $y): bool {}
    /** Unique image colors. */ public function uniqueImageColors(): bool {}
    /** Get image matte. */ public function getImageMatte(): bool {}
    /** Set image matte. */ public function setImageMatte(bool $matte): bool {}
    /** Adaptive resize image. */ public function adaptiveResizeImage( int $columns, int $rows, bool $bestfit = false, bool $legacy = false): bool {}
    /** Sketch image. */ public function sketchImage(float $radius, float $sigma, float $angle): bool {}
    /** Shade image. */ public function shadeImage(bool $gray, float $azimuth, float $elevation): bool {}
    /** Get size offset. */ public function getSizeOffset(): int {}
    /** Set size offset. */ public function setSizeOffset(int $columns, int $rows, int $offset): bool {}
    /** Adaptive blur image. */ public function adaptiveBlurImage( float $radius, float $sigma, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Contrast stretch image. */ public function contrastStretchImage( float $black_point, float $white_point, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Adaptive sharpen image. */ public function adaptiveSharpenImage( float $radius, float $sigma, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Random threshold image. */ public function randomThresholdImage( float $low, float $high, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Round corners image. */ public function roundCornersImage( float $x_rounding, float $y_rounding, float $stroke_width = 10, float $displace = 5, float $size_correction = -6): bool {}
    /** Round corners. */ public function roundCorners( float $x_rounding, float $y_rounding, float $stroke_width = 10, float $displace = 5, float $size_correction = -6): bool {}
    /** Set iterator index. */ public function setIteratorIndex(int $index): bool {}
    /** Get iterator index. */ public function getIteratorIndex(): int {}
    /** Transform image. */ public function transformImage(string $crop, string $geometry): Imagick {}
    /** Set image opacity. */ public function setImageOpacity(float $opacity): bool {}
    /** Set image alpha. */ public function setImageAlpha(float $alpha): bool {}
    /** Ordered posterize image. */ public function orderedPosterizeImage( string $threshold_map, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Polaroid with text and method. */ public function polaroidWithTextAndMethod( ImagickDraw $settings, float $angle, string $caption, int $method ): bool {}
    /** Polaroid image. */ public function polaroidImage(ImagickDraw $settings, float $angle): bool {}
    /** Get image property. */ public function getImageProperty(string $name): string {}
    /** Set image property. */ public function setImageProperty(string $name, string $value): bool {}
    /** Delete image property. */ public function deleteImageProperty(string $name): bool {}
    /** Identify format. */ public function identifyFormat(string $format): string {}
    /** Set image interpolate method. */ public function setImageInterpolateMethod(int $method): bool {}
    /** Get image interpolate method. */ public function getImageInterpolateMethod(): int {}
    /** Linear stretch image. */ public function linearStretchImage(float $black_point, float $white_point): bool {}
    /** Get image length. */ public function getImageLength(): int {}
    /** Extent image. */ public function extentImage(int $width, int $height, int $x, int $y): bool {}
    /** Get image orientation. */ public function getImageOrientation(): int {}
    /** Set image orientation. */ public function setImageOrientation(int $orientation): bool {}
    /** Paint floodfill image. */ public function paintFloodfillImage( ImagickPixel|string $fill_color, float $fuzz, ImagickPixel|string $border_color, int $x, int $y, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Clut image. */ public function clutImage(Imagick $lookup_table, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Clut image with interpolate. */ public function clutImageWithInterpolate( Imagick $lookup_table, int $pixel_interpolate_method ): bool {}
    /** Get image properties. */ public function getImageProperties(string $pattern = "*", bool $include_values = true): array {}
    /** Get image profiles. */ public function getImageProfiles(string $pattern = "*", bool $include_values = true): array {}
    /** Distort image. */ public function distortImage(int $distortion, array $arguments, bool $bestfit): bool {}
    /** Write image file. */ public function writeImageFile(mixed $filehandle, ?string $format = null): bool {}
    /** Write images file. */ public function writeImagesFile(mixed $filehandle, ?string $format = null): bool {}
    /** Reset image page. */ public function resetImagePage(string $page): bool {}
    /** Set image clip mask. */ public function setImageClipMask(imagick $clip_mask): bool {}
    /** Get image clip mask. */ public function getImageClipMask(): Imagick {}
    /** Animate images. */ public function animateImages(string $x_server): bool {}
    /** Recolor image. */ public function recolorImage(array $matrix): bool {}
    /** Set font. */ public function setFont(string $font): bool {}
    /** Get font. */ public function getFont(): string {}
    /** Set point size. */ public function setPointSize(float $point_size): bool {}
    /** Get point size. */ public function getPointSize(): float {}
    /** Merge image layers. */ public function mergeImageLayers(int $layermethod): Imagick {}
    /** Set image alpha channel. */ public function setImageAlphaChannel(int $alphachannel): bool {}
    /** Floodfill paint image. */ public function floodfillPaintImage( ImagickPixel|string $fill_color, float $fuzz, ImagickPixel|string $border_color, int $x, int $y, bool $invert, ?int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Opaque paint image. */ public function opaquePaintImage( ImagickPixel|string $target_color, ImagickPixel|string $fill_color, float $fuzz, bool $invert, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Transparent paint image. */ public function transparentPaintImage( ImagickPixel|string $target_color, float $alpha, float $fuzz, bool $invert ): bool {}
    /** Liquid rescale image. */ public function liquidRescaleImage(int $width, int $height, float $delta_x, float $rigidity): bool {}
    /** Encipher image. */ public function encipherImage(string $passphrase): bool {}
    /** Decipher image. */ public function decipherImage(string $passphrase): bool {}
    /** Set gravity. */ public function setGravity(int $gravity): bool {}
    /** Get gravity. */ public function getGravity(): int {}
    /** Get image channel range. */ public function getImageChannelRange(int $channel): array {}
    /** Get image alpha channel. */ public function getImageAlphaChannel(): bool {}
    /** Get image channel distortions. */ public function getImageChannelDistortions( Imagick $reference_image, int $metric, int $channel = Imagick::CHANNEL_DEFAULT ): float {}
    /** Set image gravity. */ public function setImageGravity(int $gravity): bool {}
    /** Get image gravity. */ public function getImageGravity(): int {}
    /** Import image pixels. */ public function importImagePixels( int $x, int $y, int $width, int $height, string $map, int $pixelstorage, array $pixels): bool {}
    /** Deskew image. */ public function deskewImage(float $threshold): bool {}
    /** Segment image. */ public function segmentImage( int $colorspace, float $cluster_threshold, float $smooth_threshold, bool $verbose = false ): bool {}
    /** Sparse color image. */ public function sparseColorImage( int $sparsecolormethod, array $arguments, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Remap image. */ public function remapImage(Imagick $replacement, int $dither_method): bool {}
    /** Hough line image. */ public function houghLineImage(int $width, int $height, float $threshold): bool {}
    /** Export image pixels. */ public function exportImagePixels( int $x, int $y, int $width, int $height, string $map, int $pixelstorage ): array {}
    /** Get image channel kurtosis. */ public function getImageChannelKurtosis(int $channel = Imagick::CHANNEL_DEFAULT): array {}
    /** Function image. */ public function functionImage( int $function, array $parameters, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Transform image colorspace. */ public function transformImageColorspace(int $colorspace): bool {}
    /** Hald clut image. */ public function haldClutImage(Imagick $clut, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Auto level image. */ public function autoLevelImage(int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Blue shift image. */ public function blueShiftImage(float $factor = 1.5): bool {}
    /** Get image artifact. */ public function getImageArtifact(string $artifact): string|null {}
    /** Set image artifact. */ public function setImageArtifact(string $artifact, string|null $value): bool {}
    /** Delete image artifact. */ public function deleteImageArtifact(string $artifact): bool {}
    /** Get colorspace. */ public function getColorspace(): int {}
    /** Set colorspace. */ public function setColorspace(int $colorspace): bool {}
    /** Clamp image. */ public function clampImage(int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Smush images. */ public function smushImages(bool $stack, int $offset): Imagick {}
    /** Create an instance. */ public function __construct(string|array|int|float|null $files = null) {}
    /** To string. */ public function __toString(): string {}
    /** Count. */ public function count(int $mode = 0): int {}
    /** Get pixel iterator. */ public function getPixelIterator(): ImagickPixelIterator {}
    /** Get pixel region iterator. */ public function getPixelRegionIterator(int $x, int $y, int $columns, int $rows): ImagickPixelIterator {}
    /** Read image. */ public function readImage(string $filename): bool {}
    /** Read images. */ public function readImages(array $filenames): bool {}
    /** Read image blob. */ public function readImageBlob(string $image, ?string $filename = null): bool {}
    /** Set image format. */ public function setImageFormat(string $format): bool {}
    /** Scale image. */ public function scaleImage(int $columns, int $rows, bool $bestfit = false, bool $legacy = false): bool {}
    /** Write image. */ public function writeImage(?string $filename = null): bool {}
    /** Write images. */ public function writeImages(string $filename, bool $adjoin): bool {}
    /** Blur image. */ public function blurImage(float $radius, float $sigma, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Thumbnail image. */ public function thumbnailImage( ?int $columns, ?int $rows, bool $bestfit = false, bool $fill = false, bool $legacy = false): bool {}
    /** Crop thumbnail image. */ public function cropThumbnailImage(int $width, int $height, bool $legacy = false): bool {}
    /** Get image filename. */ public function getImageFilename(): string {}
    /** Set image filename. */ public function setImageFilename(string $filename): bool {}
    /** Get image format. */ public function getImageFormat(): string {}
    /** Get image mime type. */ public function getImageMimeType(): string {}
    /** Remove image. */ public function removeImage(): bool {}
    /** Destroy. */ public function destroy(): bool {}
    /** Clear. */ public function clear(): bool {}
    /** Clone. */ public function clone(): Imagick {}
    /** Get image size. */ public function getImageSize(): int {}
    /** Get image blob. */ public function getImageBlob(): string {}
    /** Get images blob. */ public function getImagesBlob(): string {}
    /** Set first iterator. */ public function setFirstIterator(): bool {}
    /** Set last iterator. */ public function setLastIterator(): bool {}
    /** Reset iterator. */ public function resetIterator(): void {}
    /** Previous image. */ public function previousImage(): bool {}
    /** Next image. */ public function nextImage(): bool {}
    /** Has previous image. */ public function hasPreviousImage(): bool {}
    /** Has next image. */ public function hasNextImage(): bool {}
    /** Set image index. */ public function setImageIndex(int $index): bool {}
    /** Get image index. */ public function getImageIndex(): int {}
    /** Comment image. */ public function commentImage(string $comment): bool {}
    /** Crop image. */ public function cropImage(int $width, int $height, int $x, int $y): bool {}
    /** Label image. */ public function labelImage(string $label): bool {}
    /** Get image geometry. */ public function getImageGeometry(): array {}
    /** Draw image. */ public function drawImage(ImagickDraw $drawing): bool {}
    /** Set image compression quality. */ public function setImageCompressionQuality(int $quality): bool {}
    /** Get image compression quality. */ public function getImageCompressionQuality(): int {}
    /** Set image compression. */ public function setImageCompression(int $compression): bool {}
    /** Get image compression. */ public function getImageCompression(): int {}
    /** Annotate image. */ public function annotateImage( ImagickDraw $settings, float $x, float $y, float $angle, string $text ): bool {}
    /** Composite image. */ public function compositeImage( Imagick $composite_image, int $composite, int $x, int $y, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Modulate image. */ public function modulateImage(float $brightness, float $saturation, float $hue): bool {}
    /** Get image colors. */ public function getImageColors(): int {}
    /** Montage image. */ public function montageImage( ImagickDraw $settings, string $tile_geometry, string $thumbnail_geometry, int $monatgemode, string $frame ): Imagick {}
    /** Identify image. */ public function identifyImage(bool $append_raw_output = false): array {}
    /** Threshold image. */ public function thresholdImage(float $threshold, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Adaptive threshold image. */ public function adaptiveThresholdImage(int $width, int $height, int $offset): bool {}
    /** Black threshold image. */ public function blackThresholdImage(ImagickPixel|string $threshold_color): bool {}
    /** White threshold image. */ public function whiteThresholdImage(ImagickPixel|string $threshold_color): bool {}
    /** Append images. */ public function appendImages(bool $stack): Imagick {}
    /** Charcoal image. */ public function charcoalImage(float $radius, float $sigma): bool {}
    /** Normalize image. */ public function normalizeImage(int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Oil paint image with sigma. */ public function oilPaintImageWithSigma(float $radius, float $sigma): bool {}
    /** Oil paint image. */ public function oilPaintImage(float $radius): bool {}
    /** Posterize image. */ public function posterizeImage(int $levels, bool $dither): bool {}
    /** Radial blur image. */ public function radialBlurImage(float $angle, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Raise image. */ public function raiseImage(int $width, int $height, int $x, int $y, bool $raise): bool {}
    /** Resample image. */ public function resampleImage(float $x_resolution, float $y_resolution, int $filter, float $blur): bool {}
    /** Resize image. */ public function resizeImage( int $columns, int $rows, int $filter, float $blur, bool $bestfit = false, bool $legacy = false): bool {}
    /** Roll image. */ public function rollImage(int $x, int $y): bool {}
    /** Rotate image. */ public function rotateImage(ImagickPixel|string $background_color, float $degrees): bool {}
    /** Sample image. */ public function sampleImage(int $columns, int $rows): bool {}
    /** Solarize image. */ public function solarizeImage(int $threshold): bool {}
    /** Shadow image. */ public function shadowImage(float $opacity, float $sigma, int $x, int $y): bool {}
    /** Set image attribute. */ public function setImageAttribute(string $key, string $value): bool {}
    /** Set image background color. */ public function setImageBackgroundColor(ImagickPixel|string $background_color): bool {}
    /** Set image channel mask. */ public function setImageChannelMask(int $channel): int {}
    /** Set image compose. */ public function setImageCompose(int $compose): bool {}
    /** Set image delay. */ public function setImageDelay(int $delay): bool {}
    /** Set image depth. */ public function setImageDepth(int $depth): bool {}
    /** Set image gamma. */ public function setImageGamma(float $gamma): bool {}
    /** Set image iterations. */ public function setImageIterations(int $iterations): bool {}
    /** Set image matte color. */ public function setImageMatteColor(ImagickPixel|string $matte_color): bool {}
    /** Set image page. */ public function setImagePage(int $width, int $height, int $x, int $y): bool {}
    /** Set image progress monitor. */ public function setImageProgressMonitor(string $filename): bool {}
    /** Set progress monitor. */ public function setProgressMonitor(callable $callback): bool {}
    /** Set image resolution. */ public function setImageResolution(float $x_resolution, float $y_resolution): bool {}
    /** Set image scene. */ public function setImageScene(int $scene): bool {}
    /** Set image ticks per second. */ public function setImageTicksPerSecond(int $ticks_per_second): bool {}
    /** Set image type. */ public function setImageType(int $image_type): bool {}
    /** Set image units. */ public function setImageUnits(int $units): bool {}
    /** Sharpen image. */ public function sharpenImage(float $radius, float $sigma, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Shave image. */ public function shaveImage(int $columns, int $rows): bool {}
    /** Shear image. */ public function shearImage(ImagickPixel|string $background_color, float $x_shear, float $y_shear): bool {}
    /** Splice image. */ public function spliceImage(int $width, int $height, int $x, int $y): bool {}
    /** Ping image. */ public function pingImage(string $filename): bool {}
    /** Read image file. */ public function readImageFile(mixed $filehandle, ?string $filename = null): bool {}
    /** Display image. */ public function displayImage(string $servername): bool {}
    /** Display images. */ public function displayImages(string $servername): bool {}
    /** Spread image. */ public function spreadImage(float $radius): bool {}
    /** Spread image with method. */ public function spreadImageWithMethod( float $radius, int $interpolate_method ): bool {}
    /** Swirl image. */ public function swirlImage(float $degrees): bool {}
    /** Swirl image with method. */ public function swirlImageWithMethod( float $degrees, int $interpolate_method ): bool {}
    /** Strip image. */ public function stripImage(): bool {}
    /** Query formats. */ public static function queryFormats(string $pattern = "*"): array {}
    /** Query fonts. */ public static function queryFonts(string $pattern = "*"): array {}
    /** Query font metrics. */ public function queryFontMetrics(ImagickDraw $settings, string $text, ?bool $multiline = null): array {}
    /** Stegano image. */ public function steganoImage(Imagick $watermark, int $offset): Imagick {}
    /** Add noise image. */ public function addNoiseImage(int $noise, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Add noise image with attenuate. */ public function addNoiseImageWithAttenuate( int $noise, float $attenuate, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Motion blur image. */ public function motionBlurImage( float $radius, float $sigma, float $angle, int $channel = Imagick::CHANNEL_DEFAULT ):bool {}
    /** Mosaic images. */ public function mosaicImages(): Imagick {}
    /** Morph images. */ public function morphImages(int $number_frames): Imagick {}
    /** Minify image. */ public function minifyImage(): bool {}
    /** Affine transform image. */ public function affineTransformImage(ImagickDraw $settings): bool {}
    /** Average images. */ public function averageImages(): Imagick {}
    /** Border image. */ public function borderImage( ImagickPixel|string $border_color, int $width, int $height ): bool {}
    /** Border image with composite. */ public function borderImageWithComposite( ImagickPixel|string $border_color, int $width, int $height, int $composite ): bool {}
    /** Calculate crop. */ public static function calculateCrop( int $original_width, int $original_height, int $desired_width, int $desired_height, bool $legacy = false): array {}
    /** Chop image. */ public function chopImage(int $width, int $height, int $x, int $y): bool {}
    /** Clip image. */ public function clipImage(): bool {}
    /** Clip path image. */ public function clipPathImage(string $pathname, bool $inside): bool {}
    /** Clip image path. */ public function clipImagePath(string $pathname, bool $inside): void {}
    /** Coalesce images. */ public function coalesceImages(): Imagick {}
    /** Color floodfill image. */ public function colorFloodfillImage( ImagickPixel|string $fill_color, float $fuzz, ImagickPixel|string $border_color, int $x, int $y ): bool {}
    /** Colorize image. */ public function colorizeImage( ImagickPixel|string $colorize_color, ImagickPixel|string|false $opacity_color, ?bool $legacy = false ): bool {}
    /** Compare image channels. */ public function compareImageChannels(Imagick $reference, int $channel, int $metric): array {}
    /** Compare images. */ public function compareImages(Imagick $reference, int $metric): array {}
    /** Contrast image. */ public function contrastImage(bool $sharpen): bool {}
    /** Combine images. */ public function combineImages(int $colorspace): Imagick {}
    /** Convolve image. */ public function convolveImage(ImagickKernel $kernel, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Cycle colormap image. */ public function cycleColormapImage(int $displace): bool {}
    /** Deconstruct images. */ public function deconstructImages(): Imagick {}
    /** Despeckle image. */ public function despeckleImage(): bool {}
    /** Edge image. */ public function edgeImage(float $radius): bool {}
    /** Emboss image. */ public function embossImage(float $radius, float $sigma): bool {}
    /** Enhance image. */ public function enhanceImage(): bool {}
    /** Equalize image. */ public function equalizeImage(): bool {}
    /** Evaluate image. */ public function evaluateImage(int $evaluate, float $constant, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Evaluate images. */ public function evaluateImages(int $evaluate): Imagick {}
    /** Flatten images. */ public function flattenImages(): Imagick {}
    /** Flip image. */ public function flipImage(): bool {}
    /** Flop image. */ public function flopImage(): bool {}
    /** Forward fourier transform image. */ public function forwardFourierTransformImage(bool $magnitude): bool {}
    /** Frame image. */ public function frameImage( ImagickPixel|string $matte_color, int $width, int $height, int $inner_bevel, int $outer_bevel ): bool {}
    /** Frame image with composite. */ public function frameImageWithComposite( ImagickPixel|string $matte_color, int $width, int $height, int $inner_bevel, int $outer_bevel, int $composite ): bool {}
    /** Fx image. */ public function fxImage(string $expression, int $channel = Imagick::CHANNEL_DEFAULT): Imagick {}
    /** Gamma image. */ public function gammaImage(float $gamma, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Gaussian blur image. */ public function gaussianBlurImage(float $radius, float $sigma, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Get image attribute. */ public function getImageAttribute(string $key): string {}
    /** Get image background color. */ public function getImageBackgroundColor(): ImagickPixel {}
    /** Get image blue primary. */ public function getImageBluePrimary(): array {}
    /** Get image border color. */ public function getImageBorderColor(): ImagickPixel {}
    /** Get image channel depth. */ public function getImageChannelDepth(int $channel): int {}
    /** Get image channel distortion. */ public function getImageChannelDistortion(Imagick $reference, int $channel, int $metric): float {}
    /** Get image channel extrema. */ public function getImageChannelExtrema(int $channel): array {}
    /** Get image channel mean. */ public function getImageChannelMean(int $channel): array {}
    /** Get image channel statistics. */ public function getImageChannelStatistics(): array {}
    /** Get image colormap color. */ public function getImageColormapColor(int $index): ImagickPixel {}
    /** Get image colorspace. */ public function getImageColorspace(): int {}
    /** Get image compose. */ public function getImageCompose(): int {}
    /** Get image delay. */ public function getImageDelay(): int {}
    /** Get image depth. */ public function getImageDepth(): int {}
    /** Get image distortion. */ public function getImageDistortion(Imagick $reference, int $metric): float {}
    /** Get image extrema. */ public function getImageExtrema(): array {}
    /** Get image dispose. */ public function getImageDispose(): int {}
    /** Get image gamma. */ public function getImageGamma(): float {}
    /** Get image green primary. */ public function getImageGreenPrimary(): array {}
    /** Get image height. */ public function getImageHeight(): int {}
    /** Get image histogram. */ public function getImageHistogram(): array {}
    /** Get image interlace scheme. */ public function getImageInterlaceScheme(): int {}
    /** Get image iterations. */ public function getImageIterations(): int {}
    /** Get image matte color. */ public function getImageMatteColor(): ImagickPixel {}
    /** Get image page. */ public function getImagePage(): array {}
    /** Get image pixel color. */ public function getImagePixelColor(int $x, int $y): ImagickPixel {}
    /** Set image pixel color. */ public function setImagePixelColor(int $x, int $y, ImagickPixel|string $color): ImagickPixel {}
    /** Get image profile. */ public function getImageProfile(string $name): string {}
    /** Get image red primary. */ public function getImageRedPrimary(): array {}
    /** Get image rendering intent. */ public function getImageRenderingIntent(): int {}
    /** Get image resolution. */ public function getImageResolution(): array {}
    /** Get image scene. */ public function getImageScene(): int {}
    /** Get image signature. */ public function getImageSignature(): string {}
    /** Get image ticks per second. */ public function getImageTicksPerSecond(): int {}
    /** Get image type. */ public function getImageType(): int {}
    /** Get image units. */ public function getImageUnits(): int {}
    /** Get image virtual pixel method. */ public function getImageVirtualPixelMethod(): int {}
    /** Get image white point. */ public function getImageWhitePoint(): array {}
    /** Get image width. */ public function getImageWidth(): int {}
    /** Get number images. */ public function getNumberImages(): int {}
    /** Get image total ink density. */ public function getImageTotalInkDensity(): float {}
    /** Get image region. */ public function getImageRegion(int $width, int $height, int $x, int $y): Imagick {}
    /** Implode image. */ public function implodeImage(float $radius): bool {}
    /** Implode image with method. */ public function implodeImageWithMethod( float $radius, int $pixel_interpolate_method ): bool {}
    /** Inverse fourier transform image. */ public function inverseFourierTransformImage(Imagick $complement, bool $magnitude): bool {}
    /** Level image. */ public function levelImage( float $black_point, float $gamma, float $white_point, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Magnify image. */ public function magnifyImage(): bool {}
    /** Map image. */ public function mapImage(imagick $map, bool $dither): bool {}
    /** Matte floodfill image. */ public function matteFloodfillImage( float $alpha, float $fuzz, ImagickPixel|string $border_color, int $x, int $y ): bool {}
    /** Median filter image. */ public function medianFilterImage(float $radius): bool {}
    /** Negate image. */ public function negateImage(bool $gray, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Paint opaque image. */ public function paintOpaqueImage( ImagickPixel|string $target_color, ImagickPixel|string $fill_color, float $fuzz, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Paint transparent image. */ public function paintTransparentImage(ImagickPixel|string $target_color, float $alpha, float $fuzz): bool {}
    /** Preview images. */ public function previewImages(int $preview): bool {}
    /** Profile image. */ public function profileImage(string $name, ?string $profile): bool {}
    /** Quantize image. */ public function quantizeImage( int $number_colors, int $colorspace, int $tree_depth, bool $dither, bool $measure_error ): bool {}
    /** Quantize images. */ public function quantizeImages( int $number_colors, int $colorspace, int $tree_depth, bool $dither, bool $measure_error): bool {}
    /** Reduce noise image. */ public function reduceNoiseImage(float $radius): bool {}
    /** Remove image profile. */ public function removeImageProfile(string $name): string {}
    /** Separate image channel. */ public function separateImageChannel(int $channel): bool {}
    /** Sepia tone image. */ public function sepiaToneImage(float $threshold): bool {}
    /** Set image bias. */ public function setImageBias(float $bias): bool {}
    /** Set image bias quantum. */ public function setImageBiasQuantum(string $bias): void {}
    // TODO(verify): The optional z coordinate models ImageMagick 6/7 arity differences.
    /** Set image blue primary. */ public function setImageBluePrimary(float $x, float $y, float $z = 0.0): bool {}
    /** Set image border color. */ public function setImageBorderColor(ImagickPixel|string $border_color): bool {}
    /** Set image channel depth. */ public function setImageChannelDepth(int $channel, int $depth): bool {}
    /** Set image colormap color. */ public function setImageColormapColor(int $index, ImagickPixel|string $color): bool {}
    /** Set image colorspace. */ public function setImageColorspace(int $colorspace): bool {}
    /** Set image dispose. */ public function setImageDispose(int $dispose): bool {}
    /** Set image extent. */ public function setImageExtent(int $columns, int $rows): bool {}
    /** Set image green primary. */ public function setImageGreenPrimary(float $x, float $y, float $z = 0.0): bool {}
    /** Set image interlace scheme. */ public function setImageInterlaceScheme(int $interlace): bool {}
    /** Set image profile. */ public function setImageProfile(string $name, string $profile): bool {}
    /** Set image red primary. */ public function setImageRedPrimary(float $x, float $y, float $z = 0.0): bool {}
    /** Set image rendering intent. */ public function setImageRenderingIntent(int $rendering_intent): bool {}
    /** Set image virtual pixel method. */ public function setImageVirtualPixelMethod(int $method): bool {}
    /** Set image white point. */ public function setImageWhitePoint(float $x, float $y, float $z = 0.0): bool {}
    /** Sigmoidal contrast image. */ public function sigmoidalContrastImage( bool $sharpen, float $alpha, float $beta, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Stereo image. */ public function stereoImage(Imagick $offset_image): bool {}
    /** Texture image. */ public function textureImage(Imagick $texture): Imagick {}
    /** Tint image. */ public function tintImage( ImagickPixel|string $tint_color, ImagickPixel|string $opacity_color, bool $legacy = false ): bool {}
    /** Unsharp mask image. */ public function unsharpMaskImage( float $radius, float $sigma, float $amount, float $threshold, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Get image. */ public function getImage(): Imagick {}
    /** Add image. */ public function addImage(Imagick $image): bool {}
    /** Set image. */ public function setImage(Imagick $image): bool {}
    /** New image. */ public function newImage( int $columns, int $rows, ImagickPixel|string $background_color, ?string $format = null ): bool {}
    /** New pseudo image. */ public function newPseudoImage(int $columns, int $rows, string $pseudo_format): bool {}
    /** Get compression. */ public function getCompression(): int {}
    /** Get compression quality. */ public function getCompressionQuality(): int {}
    /** Get copyright. */ public static function getCopyright(): string {}
    /** Get configure options. */ public static function getConfigureOptions(string $pattern = "*"): array {}
    /** Get features. */ public static function getFeatures(): string {}
    /** Get filename. */ public function getFilename(): string {}
    /** Get format. */ public function getFormat(): string {}
    /** Get home url. */ public static function getHomeURL(): string {}
    /** Get interlace scheme. */ public function getInterlaceScheme(): int {}
    /** Get option. */ public function getOption(string $key): string {}
    /** Get package name. */ public static function getPackageName(): string {}
    /** Get page. */ public function getPage(): array {}
    /** Get quantum. */ public static function getQuantum(): int {}
    /** Get hdri enabled. */ public static function getHdriEnabled(): bool {}
    /** Get quantum depth. */ public static function getQuantumDepth(): array {}
    /** Get quantum range. */ public static function getQuantumRange(): array {}
    /** Get release date. */ public static function getReleaseDate(): string {}
    /** Get resource. */ public static function getResource(int $type): int {}
    /** Get resource limit. */ public static function getResourceLimit(int $type): float {}
    /** Get sampling factors. */ public function getSamplingFactors(): array {}
    /** Get size. */ public function getSize(): array {}
    /** Get version. */ public static function getVersion(): array {}
    /** Set background color. */ public function setBackgroundColor(ImagickPixel|string $background_color): bool {}
    /** Set compression. */ public function setCompression(int $compression): bool {}
    /** Set compression quality. */ public function setCompressionQuality(int $quality): bool {}
    /** Set filename. */ public function setFilename(string $filename): bool {}
    /** Set format. */ public function setFormat(string $format): bool {}
    /** Set interlace scheme. */ public function setInterlaceScheme(int $interlace): bool {}
    /** Set option. */ public function setOption(string $key, string $value): bool {}
    /** Set page. */ public function setPage(int $width, int $height, int $x, int $y): bool {}
    /** Set resource limit. */ public static function setResourceLimit(int $type, int $limit): bool {}
    /** Set resolution. */ public function setResolution(float $x_resolution, float $y_resolution): bool {}
    /** Set sampling factors. */ public function setSamplingFactors(array $factors): bool {}
    /** Set size. */ public function setSize(int $columns, int $rows): bool {}
    /** Set type. */ public function setType(int $imgtype): bool {}
    /** Key. */ public function key(): int {}
    /** Next. */ public function next(): void {}
    /** Rewind. */ public function rewind(): void {}
    /** Valid. */ public function valid(): bool {}
    /** Current. */ public function current(): Imagick {}
    /** Brightness contrast image. */ public function brightnessContrastImage( float $brightness, float $contrast, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Color matrix image. */ public function colorMatrixImage(array $color_matrix): bool {}
    /** Selective blur image. */ public function selectiveBlurImage( float $radius, float $sigma, float $threshold, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Rotational blur image. */ public function rotationalBlurImage(float $angle, int $channel = Imagick::CHANNEL_DEFAULT): bool {}
    /** Statistic image. */ public function statisticImage( int $type, int $width, int $height, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Subimage match. */ public function subimageMatch(Imagick $image, &$offset = null, &$similarity = null, float $threshold = 0.0, int $metric = 0): Imagick {}
    /** Similarity image. */ public function similarityImage(Imagick $image, &$offset = null, &$similarity = null, float $threshold = 0.0, int $metric = 0): Imagick {}
    /** Set registry. */ public static function setRegistry(string $key, string $value): bool {}
    /** Get registry. */ public static function getRegistry(string $key): string|false {}
    /** List registry. */ public static function listRegistry(): array {}
    /** Morphology. */ public function morphology( int $morphology, int $iterations, ImagickKernel $kernel, int $channel = Imagick::CHANNEL_DEFAULT ): bool {}
    /** Filter. */ public function filter(ImagickKernel $kernel, int $channel = Imagick::CHANNEL_UNDEFINED): bool {}
    /** Set antialias. */ public function setAntialias(bool $antialias): void {}
    /** Get antialias. */ public function getAntialias(): bool {}
    /** Color decision list image. */ public function colorDecisionListImage(string $color_correction_collection): bool {}
    /** Optimize image transparency. */ public function optimizeImageTransparency(): void {}
    /** Auto gamma image. */ public function autoGammaImage(?int $channel = Imagick::CHANNEL_ALL): void {}
    /** Auto orient. */ public function autoOrient(): void {}
    /** Auto orientate. */ public function autoOrientate(): void {}
    /** Composite image gravity. */ public function compositeImageGravity(Imagick $image, int $composite_constant, int $gravity): bool {}
    /** Local contrast image. */ public function localContrastImage(float $radius, float $strength): bool {}
    /** Identify image type. */ public function identifyImageType(): int {}
    /** Get image mask. */ public function getImageMask(int $pixelmask): ?Imagick {}
    /** Set image mask. */ public function setImageMask(Imagick $clip_mask, int $pixelmask): void {}
    /** Canny edge image. */ public function cannyEdgeImage( float $radius, float $sigma, float $lower_percent, float $upper_percent ): bool {}
    /** Set seed. */ public static function setSeed(int $seed): void {}
    /** Wavelet denoise image. */ public function waveletDenoiseImage(float $threshold, float $softness): bool {}
    /** Mean shift image. */ public function meanShiftImage( int $width, int $height, float $color_distance ): bool {}
    /** Kmeans image. */ public function kmeansImage( int $number_colors, int $max_iterations, float $tolerance ): bool {}
    /** Range threshold image. */ public function rangeThresholdImage( float $low_black, float $low_white, float $high_white, float $high_black ): bool {}
    /** Auto threshold image. */ public function autoThresholdImage(int $auto_threshold_method): bool {}
    /** Bilateral blur image. */ public function bilateralBlurImage( float $radius, float $sigma, float $intensity_sigma, float $spatial_sigma ): bool {}
    /** Clahe image. */ public function claheImage( int $width, int $height, int $number_bins, float $clip_limit ): bool {}
    /** Channel fx image. */ public function channelFxImage(string $expression): Imagick {}
    /** Color threshold image. */ public function colorThresholdImage( ImagickPixel|string $start_color, ImagickPixel|string $stop_color ): bool {}
    /** Complex images. */ public function complexImages(int $complex_operator): Imagick {}
    /** Interpolative resize image. */ public function interpolativeResizeImage( int $columns, int $rows, int $interpolate ): bool {}
    /** Level image colors. */ public function levelImageColors( ImagickPixel|string $black_color, ImagickPixel|string $white_color, bool $invert ): bool {}
    /** Levelize image. */ public function levelizeImage( float $black_point, float $gamma, float $white_point ): bool {}
    /** Ordered dither image. */ public function orderedDitherImage(string $dither_format): bool {}
    /** White balance image. */ public function whiteBalanceImage(): bool {}
    /** Delete option. */ public function deleteOption(string $option): bool {}
    /** Get background color. */ public function getBackgroundColor(): ImagickPixel {}
    /** Get image artifacts. */ public function getImageArtifacts(string $pattern = "*"): array {}
    /** metric - MetricType_ */ public function getImageKurtosis(): array {}
    /** return [$mean, $standard_deviation] */ public function getImageMean(): array {}
    /** return [minima, float $maxima] */ public function getImageRange(): array {}
    /** return int - PixelInterpolateMethod */ public function getInterpolateMethod(): int {}
    /** return string[] */ public function getOptions(string $pattern = "*"): array {}
    /** return int OrientationType */ public function getOrientation(): int {}
    /** return [x: float, y: float] */ public function getResolution(): array {}
    /** return ImageType */ public function getType(): int {}
    /** Polynomial image. */ public function polynomialImage(array $terms): bool {}
    /** Set depth. */ public function setDepth(int $depth): bool {}
    /** Set extract. */ public function setExtract(string $geometry): bool {}
    /** int $method - PixelInterpolateMethod */ public function setInterpolateMethod(int $method): bool {}
    /** Set orientation. */ public function setOrientation(int $orientation): bool {}
}

/** ImageMagick operation failed. */
class ImagickException extends Exception {}
/** ImageMagick operation failed. */
class ImagickDrawException extends Exception {}
/** ImageMagick operation failed. */
class ImagickPixelException extends Exception {}
/** ImageMagick operation failed. */
class ImagickPixelIteratorException extends Exception {}
/** ImageMagick operation failed. */
class ImagickKernelException extends Exception {}
