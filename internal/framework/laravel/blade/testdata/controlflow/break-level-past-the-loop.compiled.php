<?php $__currentLoopData = $items; $__env->addLoop($__currentLoopData); foreach($__currentLoopData as $item): $__env->incrementLoopIndices(); $loop = $__env->getLastLoop(); ?>
  <?php echo e($item); ?>

  <?php break 2; ?>
<?php endforeach; $__env->popLoop(); $loop = $__env->getLastLoop(); ?>
