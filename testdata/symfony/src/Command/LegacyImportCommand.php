<?php

namespace App\Command;

use Symfony\Component\Console\Command\Command;
use Symfony\Component\Console\Input\InputInterface;
use Symfony\Component\Console\Output\OutputInterface;

class LegacyImportCommand extends Command
{
    protected static $defaultName = 'app:import-legacy';
    protected static $defaultDescription = 'Imports data from the legacy system';

    protected function execute(InputInterface $input, OutputInterface $output): int
    {
        $output->writeln('Importing legacy data...');
        return Command::SUCCESS;
    }
}
