@foreach ($items as $item)
  @switch($item)
    @case(1)
      @break(3)
  @endswitch
@endforeach
