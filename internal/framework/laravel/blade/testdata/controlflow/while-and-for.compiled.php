<?php while($a): ?>
  <?php if($a > 1) continue; ?>
  <?php break 1; ?>
<?php endwhile; ?>
<?php for($i = 0; $i < 3; $i++): ?>
  <?php continue; ?>
<?php endfor; ?>
