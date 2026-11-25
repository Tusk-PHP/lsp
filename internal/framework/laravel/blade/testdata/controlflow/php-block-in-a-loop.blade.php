@foreach ($items as $item)
  @php
    if ($item) { break; }
    continue 2;
  @endphp
@endforeach
