@forelse ($items as $item)
  {{ $item }}
  @break
@empty
  none
  @break
@endforelse
