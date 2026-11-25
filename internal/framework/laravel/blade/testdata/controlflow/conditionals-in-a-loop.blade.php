@foreach ($items as $item)
  @if ($item)
    @continue
  @endif
  @unless ($item)
    @break(1)
  @endunless
@endforeach
