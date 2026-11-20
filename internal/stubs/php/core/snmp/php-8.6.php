<?php

// PHP 8.6 delta: Snmp functions, classes and enums added in 8.6,
// rendered from PHP 8.6.0's reflection. Classes list all their members.

namespace Snmp;

enum Mib {
    case AllowUnderscores;
    case CommentTerm;
    case Replace;
}

enum OidOutput {
    case Full;
    case Module;
    case None;
    case Numeric;
    case Suffix;
    case Ucd;
}

enum Output {
    case DontPrintUnits;
    case EnumPrint;
    case EscapeQuotes;
    case ExtendedIndex;
    case HexText;
    case NumericIndex;
    case NumericTimeticks;
    case QuickPrint;
}

enum StringOutput {
    case Ascii;
    case Guess;
    case Hex;
}
