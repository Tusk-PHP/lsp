@while ($a)
  @continue($a > 1)
  @break(1)
@endwhile
@for ($i = 0; $i < 3; $i++)
  @continue
@endfor
