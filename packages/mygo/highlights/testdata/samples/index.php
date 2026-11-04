<?php
declare(strict_types=1);

namespace App;

/** Doc */
final class Greeter
{
    public function __construct(private string $name = "world") {}

    public function greet(int $times): string
    {
        $out = [];
        foreach (range(1, $times) as $i) { $out[] = "Hello {$this->name} #$i"; }
        return implode(', ', $out) ?? null;
    }
}
