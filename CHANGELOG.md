# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Building from source needs Go 1.27** or later (was 1.25).
- A request whose parameters can't be read, or a `workspace/executeCommand` call with a missing or wrongly typed argument, now gets an `Invalid params` error naming the method and the argument, and the server log records it. Before, commands answered with an empty result, and a malformed notification was dropped without a trace.

### Fixed

- The "stopped indexing at maxIndexFiles" warning no longer appears when the workspace has exactly `maxIndexFiles` PHP files; it shows only when files were actually left out.
- A function call with no receiver, such as `area()`, is now always read as a function, found the way PHP finds it (a `use function` import, the current namespace, then the global one), by go to definition and declaration, hover, find references, rename and signature help. When no such function exists they give no answer, instead of a same-named method of some unrelated class, or of the class you're in. A bare name that isn't called no longer reaches another class's member either. When a method and a property share a name, go to definition, references and rename on `$this->area()` go to the method and on `$this->area` to the property.

## [0.16.0] - 2026-11-25

Thanks to [@stubbedev](https://github.com/stubbedev) and [@AJenbo](https://github.com/AJenbo), who contributed to this release.

### Added

- **Type hierarchy** (`textDocument/prepareTypeHierarchy`): from a class, interface, trait or enum, browse up to its parent, interfaces and traits, and down to the classes that extend or implement it, one level at a time. Deprecated types are marked.
- **`unknown-named-argument`** (error): a named argument the function, method or constructor has no parameter for, such as `new User(nmae: 'x')` or `$user->rename(nme: 'y')`, which PHP throws on. Calls to variadic functions, to classes with `__call` or `__callStatic`, with an argument unpacked by `...`, and to PHP's own functions are left alone. A function or class declared more than once, such as a helper guarded by `if (!function_exists('h'))` in two packages, is reported only when none of its declarations takes the argument, each with the methods of its own traits and parent.
- **Document links**: `require` and `include` paths built from `__DIR__`, `__FILE__`, `dirname()`, `DIRECTORY_SEPARATOR` and string literals, Laravel's path helpers (`base_path('routes/web.php')`, `resource_path('views')` and the rest, and `config_path()` or `storage_path()` alone link to their directory) and URLs in comments are clickable. Only files and directories that exist are linked, and a path built from variables never is.
- **`unused-private-constant`** (hint, shown faded): a private class constant nothing reads, found from the class's own code, its attributes (`#[AsCommand(name: self::NAME)]`) and its traits. `constant('self::X')` and `defined('self::X')` count as reads of `X`; a class that fetches constants by a computed name (`self::{$name}`, `constant($name)`) or through reflection (`getConstants()`, `getConstant()`, `ReflectionClassConstant`) isn't checked.
- **Ignore a diagnostic where it is**: a `// @tusk-ignore unknown-member` comment on the line, or on its own line above it, silences that finding; a bare `@tusk-ignore` silences everything on the line. The marker has to start the comment, so `// TODO: drop @tusk-ignore` silences nothing. In Blade views use `{{-- @tusk-ignore … --}}`; above a directive or echo that spans several lines, such as an `@include` with a multi-line array, it covers the whole of it. PHPStan and Pint findings are named `phpstan:<identifier>` and `pint:<rule>`.
- **"Ignore … on this line" quick fix** writes that comment for you, above the docblock and attributes so they stay attached. For PHPStan findings it writes PHPStan's own `@phpstan-ignore` at the end of the line, so PHPStan agrees.
- **Existing `@phpstan-ignore` comments are honoured**, so a codebase that already silences PHPStan needs no changes: `@phpstan-ignore-line` and `@phpstan-ignore-next-line` silence PHPStan's and Tusk PHP's findings on that line (not Pint's), and `@phpstan-ignore <identifier> (reason), …` silences those identifiers and the matching Tusk PHP finding (`method.notFound` → `unknown-member`, `class.notFound` → `unknown-class`, `variable.undefined` → `undefined-variable`, and 31 more).
- **Ignore diagnostics across files** with `diagnosticIgnore` rules in `.tusk-php.json`, matching by code, by a path glob read like a `.gitignore` entry (`app/Legacy`, `*.blade.php`), and by a regular expression on the message. A rule that can't be read is skipped and shown in the editor; the rest of the configuration still applies. Both kinds of suppression apply in the editor and to `tusk-php check`. See [Ignoring a finding](docs/diagnostics.md#ignoring-a-finding).
- **Go to declaration** (`textDocument/declaration`): on a method, property or constant, it opens the interface, parent class or trait that first declares it, such as the interface method a class implements or the `casts()` a Laravel model's `HasAttributes` trait declares, while go to definition keeps opening the implementation. PHP's own interfaces (`ArrayAccess`, `JsonSerializable`) are passed over for your project's declaration below them. On a method's own name, as on `public function handle()`, it opens what that class's method implements, never a same-named method of another class. It works in Blade views too.

### Changed

- **`diagnosticRules` in `.tusk-php.json` is checked**: a rule name that doesn't exist, such as `unused-imports`, or a severity other than `error`, `warning`, `info`, `hint` or `off` is reported instead of being silently treated as "on". The editor shows the error and the server runs with the default settings; `tusk-php check` and `tusk-php mcp` stop with it.
- `tusk-php check` and the MCP diagnostics tool now also run the rules the editor runs on save (`redundant-nullsafe`, `unknown-column`, `unknown-relation`), so they report what an editor shows after saving.

### Fixed

- Go to definition on the name of a method, constant or enum case where it's declared stays on it instead of jumping to a same-named member of another class.
- **`argument-count-mismatch`** no longer warns about a call to a function or class declared in more than one file when one of the declarations takes that many arguments.
- A function or class declared in two files no longer goes missing from go-to-definition, hover and completion when one of those files is edited or deleted. A class declared twice now has the traits, interfaces and parent of the declaration in use, instead of those of both, in hover, completion, go to implementation and the type hierarchy.
- **Inline variable** is offered in editors that only ask for the code action kinds the server announces: its kind, `refactor.inline`, was missing from the list.
- **Answers are right from the first request.** While the workspace is still being indexed, a class the editor asks about is read straight away from where Composer's autoload says it lives, with its parents, interfaces and traits, and a project model read this way gets its relations at once and keeps them. Only files indexing would read are loaded: `excludePaths` and `maxIndexFiles` apply, and a class asked for in another case than its file name doesn't appear twice. Before, a class or vendor helper indexed later than the request (such as `Str::of()` in a Laravel app) had no definition, hover or completion until indexing finished. On a 9,700-file Laravel app the first correct cross-file answer now comes after about 80 ms instead of 280 ms.
- **Doctrine** entity analysis no longer changes property types while a request is reading them.
- **Calling a closure stored in a variable** is now typed: after `$g = fn (int $i): Foo => new Foo();`, hover on `$g` shows `Closure(int): Foo`, `$g(1)->` completes `Foo`'s members, hover and go-to-definition work on them, and `$g(1)->nope()` is reported as `unknown-member`. Before, the variable had no type, so none of these worked.
- `tusk-php introspect` and the editor's debug dump list the same native diagnostics the editor shows: syntax errors, abstract methods in a class that isn't abstract, and calls to deprecated PHP functions were missing from them.
- **`redundant-nullsafe`** no longer reports `?->` on values that can be null. It used to flag nullable parameters (`?LoggerInterface $logger = null`), variables that a ternary may set to `null`, and chains after another `?->`; it now reports only `$this?->` and a parameter declared with a non-nullable type that its function never reassigns.
- Group and comma-separated imports, `use App\Lib\{Alpha, Beta as B};` and `use App\A, App\B;`, now import every name, including `function` and `const` items in a group. Before, only the first name of a comma list counted and a group imported nothing, so the classes were reported as `unknown-class`.
- A model's columns from an IDE helper file (`_ide_helper_models.php`) no longer disappear when the model is opened or edited, and re-reading the helper no longer lists them twice.
- **Columns from migrations** are read more faithfully: only what a migration's `up()` does counts, so a column its `down()` drops is no longer lost; `morphs()` and its variants, `foreignIdFor()`, `increments()` and the like, `renameColumn()`, `dropMorphs()`, `dropTimestamps()`, `Schema::rename()` and `Schema::connection(…)->create()` are understood; and double-quoted names and a closure parameter named other than `$table` work.
- **Symfony translation keys** now complete and go to their definition, as documented: inside `->trans('…')`, `t('…')` and `new TranslatableMessage('…')`, keys come from the project's `translations/` folder (YAML, PHP and XLIFF files), filtered by the domain argument when one is given. They were never connected before.
- **Symfony console command names** now complete and go to the command's class inside a console application's `find('…')`, `get('…')` and `has('…')`, as documented. Commands come from `#[AsCommand]` and the older `$defaultName`, and a command added or renamed while the editor is open is picked up when its file changes. Other classes' `get('…')`, such as a container's, are left alone.
- **Unknown Eloquent relations** in `with()`, `has()`, `whereHas()`, `load()` and the other relation methods are now reported on save as `unknown-relation`, as documented; the rule never ran before. To avoid false warnings it only reports a name the model has no method for at all, and stays quiet when part of the model (a parent class or a trait) isn't indexed or the model handles undeclared calls itself.
- **Unknown Eloquent columns** (`unknown-column`) are now checked on save, but only for models whose table was read from a live database, the one source that lists every column. Column lists worked out from migrations miss columns (from `morphs()`, raw SQL, a rollback's `dropColumn()`), so they no longer produce warnings. Arguments that may be a select alias (`orderBy()`, `orderByDesc()`, `latest()`, `oldest()`, `groupBy()`, `having()`), anything that isn't a plain column name (`*`, `options->locale`, `posts.title`) and calls inside comments or strings are not checked either.
- A model's database columns, relations and accessors no longer vanish from completion and hover after its file is opened or edited; they are worked out again for that file, and a relation added while editing shows up straight away.
- Every rule's severity can now be set in `diagnosticRules`, and the setting always applies. Setting `redundant-union-member` to `"warning"` used to do nothing, because the rule reported at `info`. `syntax-error`, `abstract-in-concrete`, `unknown-column` and `unknown-relation` can now be turned off or given a severity like the other rules; their defaults are unchanged.
- A `match` with a comment after its last arm, or between arms, is no longer reported as `syntax-error: malformed match arm`.
- An import written with a leading backslash, `use \App\Lib\Thing;`, now resolves like `use App\Lib\Thing;`, so `Thing` is no longer reported as `unknown-class`.
- `break(2);` and `continue(2);` are no longer reported as calls to unknown functions `break` and `continue`.
- A method named with a reserved word, such as `public function use(string $name)` or `function list($items)`, sees its own parameters again instead of reporting them as `undefined-variable`.
- Fewer false `unknown-member` and `unknown-function` warnings on real projects:
  - A class declared inside a block, such as a placeholder under `if (false)` or a polyfill under `if (!class_exists(…))`, no longer has its members checked, since the class code runs with may be another one. Symfony's MongoDB and PHPUnit bridges and Magento's Redis adapter are no longer flagged.
  - A variable written by a destructuring, such as `[$name, $route] = $route;`, no longer keeps the type it had before. A `foreach` variable is also known to take a new value on every turn of the loop.
  - Methods Laravel calls on a notification by convention (`via()`, `toMail()`, `toArray()`, `toDatabase()`, `toBroadcast()` and the rest) are no longer reported on `Illuminate\Notifications\Notification`.
  - A member call with a space or comment after `->` or `::`, as in `parent:: build()` or `$f->/* note */bindTo()`, is no longer reported as an unknown function.
  - A member after a nullsafe link, as in `$a?->post->missing()`, is now checked against the class the whole chain gives.
  - A member checked for first with `method_exists()`, `property_exists()` or `is_callable()` is no longer reported when the check is written over several lines or with the function name in another case, as in `METHOD_EXISTS($x, 'save')`.

## [0.15.0] - 2026-11-22

### Added

- **Eloquent static calls** such as `Category::where()`, `Category::find(1)` and `Category::orderBy()` are understood as Laravel runs them, on a query builder for the model: `Category::` completes every builder and query builder method, hover and go to definition reach them, and `Category::where(...)->first()` is a `?Category`. It works as soon as the project is indexed, instead of after a background pass over the models, and covers every builder method rather than a fixed list. Hover and go to definition also follow `@mixin`, so `->orderBy()` on an Eloquent query reaches the base query builder's method.
- **Eloquent local scopes:** `scopeActive($query)` and Laravel 12's `#[Scope] protected function active($query)` are called as `Category::active()` and `$query->active()`. Both complete on the model and its query builder, hover shows the scope's doc, go to definition opens it, and the call gives back the builder, so `Category::active()->first()` is a `?Category`.
- **PHP features inside Blade views** (part of [#62](https://github.com/Tusk-PHP/lsp/issues/62)). Hover, completion, go to definition, type definition, implementation, signature help, inlay hints, document highlights and rename now work on the PHP in a `.blade.php` file: inside `{{ }}` and `{!! !!}`, directive arguments such as `@foreach(...)` and `@if(...)`, `@php` blocks and bound component attributes (`:title="$post->title"`). Variables assigned or given a `/** @var */` in the view are typed. A view has no place for a `use` import, so completing a class there inserts its fully qualified name, such as `\App\Models\User`.
- **Find references and rename include Blade views.** References to a class, method, property, function or constant now list its uses in the project's views, and renaming it updates them too. Only PHP written in the view counts, never HTML text that happens to match.
- **Blade directives**: typing `@` in a view completes Laravel's and Livewire's directives, and a block directive such as `@foreach` comes with its `@endforeach`. Hover on a directive says what it does.
- **View names in Blade**: the view in `@include`, `@includeIf`, `@includeWhen`, `@includeUnless`, `@each`, `@extends` and `@component` is completed from `resources/views` and goes to its file. A view that doesn't exist is reported as `unknown-view`. Views from packages (`mail::message`), names built at runtime and `@includeIf` are never reported, and neither is any view in a project that adds view paths in `config/view.php`.
- **Sections and stacks**: `@section`, `@hasSection`, `@push`, `@prepend` and their variants complete the names the layout `@yield`s or `@stack`s, following `@extends` up the chain and into the views a layout includes. Go to definition on the name opens the `@yield` or `@stack`.
- **Unclosed Blade directives** are reported as `unclosed-directive`: an `@if` without `@endif`, an `@endforeach` with no `@foreach`, or an `@else` outside a conditional. Ending one conditional with another's closer (`@auth … @endif`) works in Blade and isn't reported, and neither is `@else` in a view that uses its own `Blade::if` directives.
- **Blade components**: after `<x-`, completion lists the app's class components (`app/View/Components`) and anonymous components (`resources/views/components`, including `card/index.blade.php` and `accordion/accordion.blade.php`). Inside the tag, attributes are completed from the class's constructor or the view's `@props`, leaving out those already set. Hover shows the component's class, view and attributes, and go to definition opens its class or view. Components a provider registers with a literal `Blade::component('alias', Alert::class)` count too.
- **Unknown Blade components** are reported as `unknown-component`. To avoid false warnings, it stays off in apps whose providers or packages register components the server can't name (such as an `anonymousComponentPath()` without prefix), and it never reports `<x-slot>`, `<x-dynamic-component>` or package components like `<x-mail::button>`.
- **Livewire 4 component locations**: views and anonymous components in the `component_locations` of `config/livewire.php` (by default `resources/views/livewire`) are found, so `@include` and `<x-…>` names there aren't reported as unknown.
- **Typed view variables in Blade**: a view knows the variables passed to it by `view()`, `View::make()`, `response()->view()` and `Route::view()`, with their data given as an array, `compact()`, an array assigned earlier in the same method, or `->with('key', $value)`, `->with([...])` and `->withKey($value)`. Each is typed from what is passed, such as `'user' => $user` giving `App\Models\User`, and a variable passed from several places gets the union of their types. Layouts a view extends and views it includes get its variables too, plus the data `@include` passes and the item of `@each`. Typing `$` lists the passed variables first, hover shows the type and where it's passed from, and go to definition opens the `'user' =>` key. A view also gets the data `View::share()` and view composers add (a composer class's `compose()`, or a closure's `->with()` calls), a Mailable's `Content(view:, with:)` data and public properties, a class component's public properties and methods, and an anonymous component's `@props` with their defaults. Members used on these variables are now checked too, so `$user->nmae` is reported as `unknown-member`.
- **Undefined variables in Blade views** are reported again where the server knows every place that renders the view and can read all the data passed: a variable none of them passes is `undefined-variable`. Blade's own variables (`$loop`, `$message`, `$errors`, `$slot`, `$attributes`, `$app`) and `@inject` count as defined. A view stays quiet when anything renders a view by a name built at runtime (`view($name)`, `@include($partial)`), when its name appears elsewhere as a string (as a Fortify view or a route name does), when some data can't be read (`view('x', $request->all())`), or when it's a component, Livewire, error or package view.
- **Directives and conditionals from service providers**: `Blade::directive()` and `Blade::if()` registered by the app or its packages are known in views. They're completed and documented with where they're registered, and go to definition opens the registration. A conditional's `@else…`, `@unless…` and `@end…` balance like `@if`, so an unclosed `@admin` is reported, while the arguments of a custom directive aren't checked as PHP.
- **Package components and views**: components and views that providers register are found, completed, hovered and opened. This covers `Blade::component()` and `Blade::components()` aliases (with prefixes), `loadViewComponentsAs()`, `componentNamespace()` (`<x-nightshade::calendar>`), `anonymousComponentPath()` and `anonymousComponentNamespace()`, and view namespaces from `loadViewsFrom()`, `View::addNamespace()` and Spatie's package tools (`->hasViews()`), with an app's published copies in `resources/views/vendor/{namespace}` taking precedence. `@include('courier::mail.welcome')` and `view('courier::…')` complete too, and a view missing from a namespace whose directories are all known is reported as `unknown-view`.
- **Flux and Filament components**: `<flux:` completes Flux's components and `<x-filament::` Filament's, each with the attributes from its `@props`, hover and go to definition to its view. Filament's own directives such as `@captureSlots` and `@filamentScripts` are known.

- **PHP 8.6 support** ([#91](https://github.com/Tusk-PHP/lsp/issues/91)). `8.6` is read from `composer.json` and configuration, and is now the default when a project doesn't say.
  - **Partial function application:** `f(?, 'x')`, `f('x', ...)` and named placeholders such as `f(?, times: ?)` type as a `Closure` of the parameters left open, so calling the result is typed, and the same goes for methods.
  - **Parameter doc comments:** a `/** … */` before a parameter, or after it before the comma, shows in hover, and a `@var` there types the parameter.
  - **`#[\Override]` on class constants and enum cases** is understood, and reported as `invalid-override` when there's nothing to override.
  - **New functions and classes:** `clamp()` (typed from its arguments), `SortDirection`, `Time\Duration`, the stream error API (`stream_last_errors()`, `StreamError` and its enums), the `Io\Poll` polling API, the URI builders, new Reflection methods, and the new intl, sodium, SNMP and OpenSSL additions. They complete and hover with their members on PHP 8.6 projects, and are reported as `builtin-unavailable` on older ones.
  - **8.6 deprecations** are reported as `deprecated` when a project's lowest PHP version is 8.6: `is_double()`, `is_integer()`, `is_long()`, `doubleval()`, `strcoll()`, `metaphone()`, `spl_classes()`, `spl_object_hash()`, `mysqli_get_charset()`, `mysqli_stmt_init()`, the `mb_ereg*` family, and `ArrayIterator`'s sort, flag and serialize methods, each with the replacement PHP suggests.

### Changed

- `tusk-php check` now checks Blade views too when given a directory or the whole project, as the editor does.

### Fixed

- A method declared only on an interface is now found on the classes that implement it, including through an abstract parent that leaves the method to its subclasses, or through an interface that extends another. Completion, hover, go-to-definition and generics work on it: with `@implements Repository<TEntity>` on the abstract parent and `@extends BaseRepository<Person>` on the class, `find()` returns `Person`.
- **Doctrine entities:** each property reads only its own attributes, so an `#[ORM\Id]` no longer makes the next property read-only, and in the model relations `tusk-php introspect` and the MCP server list, a relation no longer takes the type of the one declared just above it. A bare `#[ORM\Column]` without arguments, as Symfony's maker bundle writes for `$id`, is recognised as a column.
- A fully qualified function call inside a namespace, such as `\Vendor\Lib\helper()`, is no longer reported as `unknown-function`, and a qualified call like `Lib\helper()` resolves through `use Vendor\Lib;` as PHP does.
- Functions and classes added in PHP 8.5, such as `array_first()`, `NoDiscard` and the `Uri` classes, are reported as `builtin-unavailable` on projects that require an older PHP instead of being treated as known.
- A PHP builtin the project's PHP version or extensions don't have, such as `Redis` without `ext-redis`, is reported once, as `builtin-unavailable`, instead of also as `unknown-class` or `unknown-function`, and the message no longer ends in an empty "(PHP >= )".
- `builtin-unavailable` no longer reports an attribute's arguments as a function call (`#[\NoDiscard('…')]`, `#[\Deprecated('…')]`), and stays quiet about newer builtins in a file that checks `PHP_VERSION_ID` or `PHP_VERSION`, where they're used behind that check.
- **Laravel facade aliases are known classes.** `Route::has('login')` in a Blade view, or `Cache::get()` in a file without a namespace, is no longer reported as `unknown-class`, and hover and go to definition lead to the class the alias stands for. This covers Laravel's default aliases, the `aliases` in `config/app.php` and the ones packages register in `extra.laravel.aliases` (such as Flux's `Flux`), except packages listed under `dont-discover`. An alias whose class isn't installed is still reported.
- A method called straight on a function's result, such as `map($input)->to(CreateProjectData::class)`, now gives the method's return type, with its `@template` bound from the arguments, instead of the function's. This removes false `unknown-member` warnings on the result.
- A private method or property that only a trait the class uses calls, reads or declares as `abstract private` is no longer reported as `unused-private-method` or `unused-private-property`. A class using a trait the server can't find isn't checked.
- `#[\Override]` is accepted on a method that overrides one a parent class gets from a trait, as in tests extending Orchestra Testbench's `TestCase`. On a method that only replaces one from the class's own trait, it's still reported, as PHP does.
- **Classes from PSR-0 packages and Composer itself are known.** Dependencies that autoload with `psr-0`, such as Magento's `Zend_Db_Expr` from `magento/zend-db`, are indexed, and so are `Composer\InstalledVersions` and `Composer\Autoload\ClassLoader`, which Composer installs into every project. They're no longer reported as `unknown-class`, and hover, completion and go to definition work on them.
- **Optional packages aren't reported as unknown.** In a library that lists packages under Composer `suggest`, classes from the ones that aren't installed are no longer reported as `unknown-class`, such as Laravel's `Ably\AblyRest` and `Pusher\Pusher`. Once a suggested package is installed, its classes are checked as usual. A class checked with `class_exists(Foo::class)` (or `interface_exists`, `trait_exists`, `enum_exists`) isn't reported in that file either, just like the quoted `class_exists('Foo')`.
- **Plain PHP views don't report their variables as undefined.** A `.php` file that writes HTML or text outside `<?php ?>` and declares no class is treated as a view, as `.phtml` files already were: the variables its top level reads come from the code that includes it, often through `extract()`. This covers Laravel's console components, Symfony's `.html.php` templates and WordPress themes and feeds. Functions and closures inside a view are still checked.

## [0.14.0] - 2026-11-13

### Added

- **`tusk-php check`** reports the diagnostics your editor shows from the command line, for a whole project or the files, directories and globs you name, as text or JSON, and exits with `1` on errors or warnings. `--exclude` leaves out paths such as test directories, and `--write-baseline` and `--baseline` let CI fail only on new findings. See [docs/command-line.md](docs/command-line.md).
- **Too few arguments** is reported by `argument-count-mismatch`: `$mailer->send('to@example.test')` on a `send(string $to, string $subject)` says `Too few arguments to send(): 1 passed, at least 2 required`, which PHP would throw on. It covers functions, static calls and `->` calls whose receiver is one known class. Calls with named arguments or `...$spread`, variadic methods, classes with `__call`, methods only declared by `@method`, and `$this` inside closures aren't checked.
- **`instanceof` narrows variables** for hover, completion and diagnostics alike: inside `if ($o instanceof Foo) { … }`, after a guard such as `if (!$o instanceof Foo) { return; }`, after `assert($o instanceof Foo)`, in a `while`, an `elseif`, a `switch (true)` case or a `match (true)` arm, and after `&&`, `||` or `?` in the same expression, `$o` is a `Foo`: `$o->` completes its members and unknown members are checked against it. A `Foo|Bar` tested against `Foo` is `Bar` in the `else` branch, and a class tested against an interface it doesn't implement is both.
- **Catch variables are typed:** in `catch (NotFound $e)` hover shows `NotFound`, `$e->` completes its methods, and unknown members on it are reported. `catch (A|B $e)` gives `A|B`. When the variable already had a value before the `try`, after the `catch` it may hold either.

### Fixed

- `argument-count-mismatch` no longer reports calls inside a version check, where each branch is written for a different version of PHP or of a library: any branch of an `if`/`elseif`/`else` whose condition uses `version_compare()`, `PHP_VERSION_ID` and the other `PHP_*VERSION` constants, a class constant such as `Kernel::VERSION_ID`, or Composer's `InstalledVersions`. Laravel's `$instance->enable()` for PHPUnit releases before 12.3.4 was reported against the newer signature.
- **Class completion no longer adds a clashing import.** When the class's name is already taken in the file, by another `use`, a class declared in the file or a class in the file's namespace, it is imported under an alias made from its namespace instead, as in `use Illuminate\Foundation\Auth\User as UserAuth;` next to `use App\Models\User;`, and the alias is what gets inserted. Existing imports are never renamed, and a class that is already imported under an alias is inserted by that alias. The new `autoImport.aliasStyle` setting picks `suffix` (`UserAuth`, the default), `prefix` (`AuthUser`) or `fullyQualified` (no import, inserts `\Illuminate\Foundation\Auth\User`).
- A comment inside a parameter list, as in `function getValues(/* bool $raw = false */)`, is no longer read as a parameter, so hover and signature help show the method's real parameters.
- Hover, completion and go to definition work on a method called after a long multi-line argument list, as in a seeder's `Collection::make([ … hundreds of lines … ])->each(…)`. They stopped working once the list ran past 200 lines; it can now run to 2,000, and brackets inside its strings no longer throw it off.
- **A variable assigned in different branches** now has the types of all of them: after `if ($c) { $x = new Foo(); } else { $x = new Bar(); }`, a `switch`, a `try`/`catch`, an `if` without `else`, a loop or `$x ??= new Bar()`, hover and completion show `Foo|Bar`, and `$x->fooOnly()` is no longer reported as an unknown member on `Bar`. When one of the values can't be typed, the variable is left unknown instead of taking the last assignment's type. Go to definition on such a variable goes to the first of its classes.
- A `foreach` variable given a new value, as in `foreach ($foos as $item) { … } $item = new Bar();`, has that value's type instead of the loop's, so its members are no longer reported as unknown.
- In `$query = $query->where('a');`, the `$query` on the right has the type it had before the assignment, so it completes and is typed in hover, also inside `if` blocks and loops.
- A comment after an assignment, as in `$n = $y->count(); // see $y`, is no longer read as part of the assigned value.
- `DateInterval` properties (`$interval->days`, `->y`, `->invert` and the others) are no longer reported as unknown members.
- `$this->$name = $value;` no longer counts as an assignment to `$name`, so `unused-variable` doesn't report `$name` as assigned but never read.
- Parameters of functions, methods and closures that return by reference, as in `public function &__get($name)`, are no longer reported as undefined variables or as assigned but never read.
- **First-class callable syntax** such as `$f = $box->get(...);` or `strlen(...)` now gives a `Closure` with the callee's signature instead of what the callee returns, so `$f->bindTo(null)` is no longer reported as an unknown member, `$f->` completes `Closure`'s methods and `$f()` has the callee's return type. A spread call, `f(...$args)`, is still a call.
- The unknown-member warning no longer fires on a method or property your code checks for first, as in `if (method_exists($notification, 'toArray')) { return $notification->toArray($notifiable); }`. This also covers `property_exists()`, `is_callable([$x, 'name'])`, early returns and ternaries, on the same receiver within the same function.
- Calling a static method or reading `::class` through a property, as in `$this->type::tryFrom($value)` or `$this->related::class`, no longer reports the property name as an unknown class.
- `undefined-variable` no longer reports a static property used as the class name after `new`, as in `new static::$builder($query)`.
- A closure or arrow function parameter that has the same name as a variable outside it, as in `catch (Outer $e) { array_map(fn (Inner $e) => $e->path(), $list); }`, has its own type in unknown-member checks; members were checked against the outer variable's type before.
- Unknown-member warnings name each possible class once: `Unknown member 'nope' on 'Foo|Bar'` instead of `'Foo|Bar|Foo'`.

### Changed

- `argument-count-mismatch` no longer reports too many arguments to a method. PHP allows it, and libraries read the extra arguments with `func_get_args()` to stay compatible, as `Ulid::isValid($id, $format)` does. Too many arguments to a plain function is still reported.
- **Typing no longer waits for diagnostics:** after an edit, diagnostics are worked out in the background once you pause for about 200 ms, so completion, hover and signature help answer straight away. On a 6,700-line file, completion right after a keystroke went from about 440 ms to under 40 ms. Diagnostics still arrive for the latest text; results for text you've since changed are dropped. Diagnostics on open and on save (PHPStan, Pint) are unchanged.
- Hover on a variable shows its whole union type, `array|bool|null` instead of `?array`. A callable inside a union is shown in parentheses, as in `(callable(TValue): float)|int|null`.
- A string concatenation such as `$dir . '/' . $name` is typed as `string`.

## [0.13.0] - 2026-11-09

### Added

- **Generics** are read the way PHPStan defines them ([#68](https://github.com/Tusk-PHP/lsp/issues/68)), so hover, completion and diagnostics agree on what Laravel, Symfony, Doctrine and other PHPDoc-typed code returns:
  - **Method and function templates** bind from the call: a plain argument (`identity(new User)` is `User`), `Foo::class` for a `class-string<T>` parameter (Symfony's `$envelope->last(BusNameStamp::class)?->getBusName()`), arrays and generic objects in any argument position, named arguments and default values. `new Collection([$user])` is a `Collection<int, User>` wherever it's written. A template nothing binds takes its default (`@template T = string`), then its bound (`@template T of Model`).
  - **Inheritance**: `@extends`, `@implements` and `@use` (and their `@template-` forms) pass type arguments through every level, including `@use` written above a trait's `use` statement, as Laravel's collections do. A method without its own `@return` takes the one it overrides. `@template` on interfaces and traits is now read; it was ignored before.
  - **Iterables**: `foreach` types keys and values of `list<T>`, `non-empty-array<K, V>`, `iterable<K, V>`, generators and any object that implements `Traversable` with type arguments, such as `Collection<int, User>`. `ArrayIterator`, `ArrayObject`, `SplObjectStorage`, `WeakMap`, `Iterator`, `IteratorAggregate` and `Generator` carry their generics; `$x = yield …` is the generator's send type and `getReturn()` its return type.
  - **Shapes and offsets**: `$data['user']` is the type at that key of an array shape, `$pair[1]` the second type of a `list{…}`, and `$map[$key]` the value type of an `ArrayAccess`. Object shapes (`object{name: string}`) complete their properties. `key-of<…>`, `value-of<…>`, offset access `T['key']`, `Collection<covariant Animal>` and `Collection<*>` resolve.
  - **Callables**: `callable(User): string` and `Closure(int): User` signatures are kept. A closure passed to such a parameter gets its parameters' types, so `$users->map(fn ($u) => $u->…)` completes `User`'s members, and its return binds templates, so that `map()` gives a collection of what the closure returns. Calling a `Closure`-typed variable or an invokable object gives its return type, and `@param-closure-this` sets `$this` inside the closure.
  - **Conditional return types** pick the branch the arguments choose, including `is not`, nested conditions and conditions on templates; before, both branches were offered. A `class-string<User>` value works in `new $class` and `$class::create()`, `new<T>` resolves, and a generic `@mixin T` brings in `T`'s members.
  - **Narrowing**: after a call tagged `@phpstan-assert` (with `!Type` and `=Type`), such as PHPUnit's `assertInstanceOf()`, the variable passed has the asserted type; `@phpstan-assert-if-true` and `-if-false` apply inside `if` and `else` blocks and after an early `return`; `@param-out` types a by-reference argument after the call; `@phpstan-self-out` and `@phpstan-this-out` change the object's type.

### Changed

- **Diagnostics are 3–4× faster** on large files. The rules that check member access share what they learn about types within a run, a variable's type is worked out once per assignment instead of at every line it's used on, class members are looked up by name, and the file is tokenized once for all rules. Over Laravel's test suite a sample of 329 files went from 13.7 s to 3.4 s, and Laravel's largest test file from 2.6 s to under 0.5 s. What's reported is unchanged.
- **Completion lists over large classes are about 10× cheaper** for editors that support `completionItem/resolve` (VS Code among them): member items are sent without their documentation and detail, which are filled in when an item is selected. Editors that don't declare that support still get everything in the list.
- **PHPDoc types win when they say more** than the native type: a template, a generic, a shape, a narrower union or a subclass, as PHPStan reads them. `@return TStamp|null` on `last(): ?StampInterface` is now used. A vaguer or contradicting PHPDoc type still loses to the native one.
- `@phpstan-param`, `@phpstan-return`, `@phpstan-var`, `@phpstan-template` and their `@psalm-` forms win over the plain tags for the same parameter.
- Diagnostics such as `unknown-member` and `redundant-nullsafe` now use the same generic-aware types as hover and completion, so a member called on `Collection<int, User>`'s `first()` is checked against `User`. Unions are kept whole: a member that any class of a union declares is completed and not reported.
- Generic `@var` on local variables and on properties, and generic `@method` and `@property` types, are read with the declaring file's imports.
- `Arr::first($items)` and similar `TValue|TDefault` methods are typed as nullable when no default is passed, as PHPStan types them.

### Fixed

- **Background work can no longer leave stale results.** A large file (over 5,000 lines) is indexed in the background; when it was edited again quickly, the older version could finish last and leave outdated symbols in the index. Versions of a file are now indexed in order. Diagnostics worked out in the background (after indexing finishes, and after PHPStan and Pint on save) are no longer published once the file has changed or closed, so they can't replace newer ones.
- Messages to the editor are written one at a time, so two can never interleave on the output stream, whatever the stream is.
- `unknown-member` no longer reports a property that is only checked with `isset()`, `unset()`, `empty()` or `??`, where PHP doesn't require it to exist, or a property of a class with `#[AllowDynamicProperties]` (WordPress's upgrader skins and `WP_Taxonomy`, among others).
- **Anonymous classes** (`new class { … }`) are understood: inside one, `$this`, `self` and `static` refer to it, not the class around it, so its own members, its promoted constructor properties and what it inherits are completed and checked. This removes the false `unknown-member` warnings in Laravel's `AsCollection`-style casts, Symfony's `TimezoneType` and WordPress's `WP_Block`. Anonymous classes are never offered as class names.
- A parameter given a new value takes that value's type, as Laravel's `Sleep::duration()` does with `$duration = CarbonInterval::instance($duration)`. This applies to hover, completion and diagnostics. The declared type still applies before the new value, and where the new value may not have been assigned, as after an `if` block that assigns it, the type is treated as unknown so nothing is reported. A parameter reused as a `foreach` variable has the element type inside the loop. `$x ??= …` keeps the declared type.
- `unknown-member` no longer reports a member that the implementations of an interface, or the subclasses of an abstract class, declare. An object of such a type is always one of those, and code often calls what they add, as in Magento's `$this->getResource()->getAttribute()` on `AbstractDb` and `$item->getQtyToShip()` on `OrderItemInterface`. A member that nothing below the type declares is still reported, and so is a subclass's member called on a concrete class.
- A PHPDoc type now means what the imports of the file that declares it say, even where the caller imports another class with the same short name. Magento's `Helper::initialize()`, documented `@return Product` for `Magento\Catalog\Model\Product`, no longer takes the caller's own `Product`, so hover, completion, go-to-definition and diagnostics on the result are right. This covers `@param`, `@return`, `@var` and `@throws` (and their `@phpstan-` and `@psalm-` forms); hover now shows those types by their full name.

## [0.12.0] - 2026-11-09

### Added

- **Blade templates are read as Blade** (part of [#62](https://github.com/Tusk-PHP/lsp/issues/62)). Diagnostics on `.blade.php` files check only the PHP in them, the code inside `{{ }}`, `{!! !!}`, directive arguments, `@php` blocks and bound component attributes (`:title="$post->title"`), and report it at its place in the view.
  - Directives, `<x-…>` components, HTML and Alpine attributes such as `@click` are no longer read as PHP. In laravel/framework's own views that removes all 382 false warnings, such as `section` and `props` reported as unknown functions.
  - The variables a view receives (`$slot`, `$attributes` and the controller's data) aren't reported as undefined.
  - Unused variables aren't reported in views, since the views they include can read them.
  - Blade files no longer add symbols to the index.
- **Variable completion**: typing `$` lists the variables in scope at the cursor: the function's own variables (most recently used first), then its parameters, then `$this`, then PHP's superglobals. Variables only assigned below the cursor aren't offered, and an arrow function also sees its parent's variables.
- **Magento 2 generated classes** resolve without running `bin/magento setup:di:compile` first. `ProductFactory`, `Product\Proxy`, `Product\Interceptor`, `ProductSearchResults`, `ProductExtensionInterface` and `ProductExtension` are built from the class they're generated for, so they're no longer reported as unknown, and they get hover, completion and go-to-definition (which opens the original class). `$this->productFactory->create()` is typed as `Product`. Extension attributes from `extension_attributes.xml` aren't read yet, so their getters and setters are neither completed nor reported. Magento projects are detected from `bin/magento` or a `magento/framework` requirement in `composer.json`, or by setting `framework` to `magento`.
- Completion, hover and extension availability diagnostics for Imagick, SSH2, Redis, Memcached, APCu and igbinary. Extensions listed in Composer's `suggest` now count as available alongside `require` and `config.platform`.
- Global constants declared with `const` at file level (`const MAX_ITEMS = 10;`, also inside a namespace) are now indexed as symbols, as class constants already were. This is what makes the SSH2 constants above known.
- **`.phtml` templates** are now read as PHP: they are indexed and get diagnostics, hover, completion and navigation like `.php` files. A template is always included by other code that sets its variables, so its top-level `$block`, `$escaper` and similar are not reported as undefined; variables inside its functions and closures still are. PHPStan and Pint keep running on `.php` files only.

### Changed

- **Completion is ordered by scope**, closest first, before anything is typed and among equally good matches after:
  - names imported with `use`;
  - then the current namespace and its sub-namespaces;
  - then each level up (siblings under `App\Http`, then under `App`);
  - then the project's other root namespaces (`Database\`, `Tests\`);
  - then Composer packages;
  - then PHP's own functions, classes, keywords and types.

  After `->` and `::`, the class's own members come before its traits' and its parent's, and an overridden method is listed once.
- `maxIndexFiles` now defaults to `0`, meaning no limit, so large projects such as Magento 2 (about 25,000 files) are indexed in full instead of stopping at 10,000 files with the rest reported as unknown. Set a limit if memory is tight: all of Magento 2 takes about 600 MB. When the limit is reached, the server now logs a warning saying how to change it. The WebAssembly build keeps `10000` as its default to stay within browser memory.

### Fixed

- PHP's Reflection, date and exception classes now have all their members, each from the PHP version that added it. `ReflectionClass::getAttributes()`, `getParentClass()` and `getFileName()`, `ReflectionParameter::getType()`, `ReflectionNamedType::getName()`, `Exception::getFile()` and `DateTime::createFromFormat()` were reported as unknown members, and `DateTime::createFromTimestamp()` is now known on PHP 8.4 and later. The Windows-only `sapi_windows_*` functions are no longer reported as unknown.
- A method a class gets from a trait now takes precedence over the parent's method of the same name, as in PHP. Carbon's `Carbon::createFromFormat()` and `setTimezone()` return a Carbon, not a `DateTime`.
- In a chain, a method call finds the method even when the class also has a property of the same name, such as `$event->timezone('UTC')` on Laravel's scheduled events, which have a `$timezone` property.
- Methods that return by reference (`public function &get()`) or are named after a keyword (`public function use()`) are now read as methods. Before, the rest of the class was lost after them, so its other methods were reported as unknown and its properties as unused, as in Symfony's `FormFieldRegistry`.
- `unknown-function` no longer mistakes newer syntax for calls: `new ($class)()`, `yield from (...)`, an anonymous class with `new` on the line before `class($args)`, PHP 8.4 asymmetric visibility (`public private(set)`) and property hooks with a parameter (`set (array $value) { … }`), and DNF property types (`private (A&B)|null $x`).
- A method or function named `match()` with no return type, such as Laravel's `Str::match()` and the Eloquent relations' `match()`, is no longer read as a `match` expression, which reported "malformed match arm" syntax errors at the end of the method.
- The `deprecated` diagnostic no longer flags methods that share a name with a deprecated PHP function, such as Laravel's `$collection->each(...)` or `Arr::each(...)`, nor matches inside `foreach(`, strings or comments. Only real calls to the global `each()`, `create_function()`, `utf8_encode()` and `utf8_decode()` are reported.
- Hover now works on a method chained after a multi-line argument list, such as `->each` in `Collection::make([...])->each(...)` where the array spans several lines.
- Heredoc and nowdoc strings are now read as strings. Their contents (SQL, braces, words followed by `(`) no longer cause `unknown-function` or `unreachable-code` warnings, nor throw off where a class or method ends. Variables interpolated in a heredoc count as used.
- Diagnostics no longer look inside strings that span several lines.
- Code that checks for an optional extension before using it is no longer reported. In a file that calls `extension_loaded( 'imagick' )`, `class_exists( 'Imagick' )` or `function_exists( 'ssh2_connect' )`, the guarded classes and functions (for an extension, every name starting with it, such as `ImagickPixel` or `ssh2_exec`) aren't reported by `unknown-class`, `unknown-function` or `builtin-unavailable`.
- An enum case or constant that shares its name with a class, such as `UserRole::Client->value` next to a `Client` model, now resolves to the enum. This fixes false `unknown-member` warnings and wrong completion and hover.
- Anonymous classes (`new class extends Migration`, `new readonly class`) are no longer reported as an unknown class named `class`.
- Variables and properties named like a keyword no longer look like one to `unknown-class`: `foreach ( $new as $key => $value )` was reported as an unknown class `as`, and the same could happen with `$extends`, `$catch`, `$implements` or `$this->new`.
- Classes from packages that autoload by `classmap`, such as PHPUnit's `#[CoversClass]`, `#[Group]` and `#[DataProvider]` attributes, are now indexed instead of being reported as unknown.
- `argument-count-mismatch` no longer counts a trailing comma in a multi-line call as an extra argument.
- The global-function `deprecated` warnings now respect the `deprecated` rule's enable and severity settings.
- The global-function `deprecated` warnings now follow the project's PHP version from `composer.json`. A project that still supports PHP 7.4 is no longer told that `utf8_encode()` or `utf8_decode()` is deprecated (that happened in PHP 8.2). `each()` and `create_function()`, deprecated in PHP 7.2, are still reported.
- Text outside `<?php … ?>`, such as the HTML and `<script>` blocks in WordPress templates and admin pages, is no longer read as PHP. JavaScript like `jQuery(...)`, `new URL()` or `_.each()` no longer triggers `unknown-function`, `unknown-class`, `deprecated` or `argument-count-mismatch`, and variables set before a `?>` count as defined after the next `<?php`. A file with no `<?php` tag at all is still read as PHP.
- Functions declared inside another function, such as WordPress's `wxr_cdata()` inside `export_wp()`, are now indexed, so calls to them are no longer reported as unknown.
- `argument-count-mismatch` no longer checks a method or function declaration against a global function with the same name. For example, `public function init( $data, $site_id = 0 )` in WordPress, or a Magento constructor, was reported as "too many arguments" to a global `init()` or `__construct()`. Class names after `new` are not checked as function calls either.
- `undefined-variable` no longer reports a variable tested with `isset()`, `empty()` or `unset()`, or on the left of `??` and `??=`, since those accept variables that may not exist (`if ( ! isset( $parent_name ) )`, `$x = $maybe ?? 'default'`). Variables evaluated inside them, such as `$key` in `isset( $items[ $key ] )` or the right side of `??`, are still checked.
- Class, interface, trait, enum and function names now match regardless of case, as they do in PHP. `new StdClass()`, `$x instanceof pclzip` for a class declared `PclZip`, `ZIPARCHIVE::CHECKCONS` and `floatVal()` are no longer reported as unknown. Constants and members stay case-sensitive.
- PHP's own attributes, such as `#[AllowDynamicProperties]`, `#[SensitiveParameter]`, `#[ReturnTypeWillChange]` and `#[Override]`, are no longer reported as unknown on projects targeting an older PHP. On PHP 7, where `#[...]` is a comment, they get no diagnostic at all. On PHP 8.x, one from a newer version is a hint that it has no effect yet, instead of a `builtin-unavailable` warning. Inside a namespace, an attribute that resolves into that namespace without an import is still reported, because PHP won't treat it as its own.
- `unknown-member` no longer reports a method or property used through `$this` or `static::` when a subclass declares it. A base class can rely on its subclasses this way, as WordPress's `ftp_base` does with `_exec()` from `ftp_sockets` and `ftp_pure`. Access through `self::`, `parent::` or a variable typed as the base class is still checked.
- `unused-private-property` and `unused-private-method` no longer miss uses further down a class whose code contains `{` or `}` inside strings or comments, such as WordPress's `WP_Block_Processor`, which compares characters with `'{'`. Its `$last_json_error` property was reported unused although it is read.
- `parent::`, `self::` and `static::` are no longer reported as unknown classes. When the parent class is missing, it is reported once, where the class `extends` it, instead of at every `parent::__construct()`.
- Methods returning `static`, `$this` or `self` now give back the class they were called on everywhere, so `unknown-member` no longer reports methods of a subclass after a fluent or factory call such as `static::create()->childOnly()` or `$this->withName('x')->childOnly()`. Calls through `self::`, `parent::` and `static::` keep the calling class, as in PHP, and these keywords now resolve in any letter case.
- `self`, `static` and `parent` now work fully in the editor. Hover and go-to-definition work on `self::$prop` and `static::$prop`, and on the keywords themselves (`new self`, `parent::`, `instanceof self`). `$x = new self()`, `new static()` and `new parent()` are typed as the class they create, and `self::class` and `static::class` as `string`. Completion after `parent::`, `self::` and `static::` inside an instance method now offers instance methods, such as `parent::__construct()`, but not the parent's private members.

## [0.11.1] - 2026-11-02

### Added

- Releases now include a WebAssembly build, `tusk-php-wasip1-wasm.wasm`, with its SHA-256 in `checksums.txt`. It keeps its memory under about 448 MB by collecting garbage harder as it gets close.

## [0.11.0] - 2026-11-02

### Added

- **Full PHP 7.4 to 8.5 syntax coverage**: completion, hover, go-to-definition and diagnostics now understand `??=`, array spread, `throw` expressions, `new` in initializers, DNF types such as `(A&B)|null`, constants in traits and interfaces, typed class constants, and `clone($a, [...])`.
- **Enums are complete**: methods, constants and traits used by an enum now show up, along with what PHP gives every enum (`name`, `value`, `cases()`, `from()` and `tryFrom()`), and these no longer report as unknown members.
- **Promoted constructor properties** now complete, resolve their type and go to their declaration, as other properties do. Hover also shows when a property is `readonly`.
- **Types flow through the pipe operator**: `$x = 'a' |> make(...) |> other(...)` takes the type of the last stage, including method callables and closures with a declared return type.
- **Attributes in hover**: hover shows a symbol's attributes as written, such as `#[\Deprecated('use other')]`.
- `invalid-override` diagnostic: an error on a method with `#[\Override]` that overrides nothing. It stays quiet if a parent class or interface isn't indexed.
- `deprecated` diagnostic now also covers your own functions, methods, constants and enum cases marked `#[\Deprecated]`.
- `unused-result` diagnostic: a warning when the result of something marked `#[\NoDiscard]` is thrown away. Cast to `(void)` to do so on purpose.
- Signatures for `array_find()`, `array_find_key()`, `array_any()`, `array_all()`, `array_first()` and `array_last()`, and the PHP 8.4 lazy object methods on `ReflectionClass` and `ReflectionProperty`.

### Fixed

- `?Foo` is no longer reported as `redundant-union-member`. Only a real `?Foo|null` is, and that can't be written in valid PHP.
- `$this->` completion now includes the class's own private methods and properties.
- Union and intersection types keep their shape in hover, and completion offers the members of every class in them rather than only the first.
- `unreachable-code` now also checks functions, traits and enums, not only classes.
- A `set` hook's implicit `$value` is no longer reported as an undefined variable.
- A variable first assigned with `??=` is no longer reported as undefined.
- `unknown-member` now checks variables in namespaced files, which it used to skip without saying, and a union such as `Post|Thread` is only reported when no class in it has the member. Calling a method on a typed parameter or a chained result now reports a real typo.
- `unknown-member` no longer reports `Foo::class`, methods written in a different case, or anything `__call`, `__callStatic`, `__get`, `__set` or `@mixin` could answer, which covers Eloquent models, facades and macros, though a typo on one of those isn't reported either. It also stays quiet when a parent class isn't indexed, on builtins with no known members, inside traits, on vendor interfaces such as `Storage::disk()`'s `Filesystem`, and when the variable was tested with `instanceof`.
- The server now stops if its input stream keeps failing, instead of retrying forever and logging each failure. After 100 read errors in a row it exits with an error.
- The WebAssembly build (`make wasm`) can now run under VS Code's WASI host, where it used to fail every read of its input, and it uses little CPU while idle.

## [0.10.0] - 2026-10-30

### Added

- **Test coverage in hover cards**: methods and functions show a coverage bar with their lines and CRAP score, such as `🟡 ████████░░ 80% · 12/15 lines · CRAP 5`, from the Clover report your tests last wrote (`--coverage-clover`, or `<clover>` in `phpunit.xml`), and note when the file changed since that report. Nothing runs your tests, and nothing is shown when there's no report or the code isn't in it. Reports written inside Docker or Sail are matched automatically. Set `coverageReportPath` if the report is somewhere the server won't find it.
- `php_coverage` MCP tool: gives AI agents a file's uncovered lines and untested methods, or project totals and the least covered files.
- **Line-level coverage for editors**: new `tusk/coverageReport` and `tusk/fileCoverage` requests return a report's files with their totals, and a file's executable lines with hit counts and its methods, each marked when the file changed since the report. Coverage that isn't known comes back as `null` or simply isn't listed, so it can't read as untested. Clients that set the `coverageNotifications` initialization option also get `tusk/coverageChanged` when a report appears, changes or goes away. Advertised as `experimental.tuskCoverage`.

### Changed

- **Easier-to-read hover cards**: PHP signatures are now syntax highlighted, type names are shortened (`JsonResponse` rather than `Illuminate\Http\JsonResponse`), long signatures put each parameter on its own line, and the description comes after the signature. The code block starts with a `<?php` line, because editors only highlight PHP after an opening tag.

### Fixed

- Hovering a method's name where it's declared now shows that method, instead of a method with the same name from another class.
- Anonymous functions no longer report `Unknown function 'function'`. The same goes for other keywords written before a parenthesis, such as `use (`, `fn (`, `return (` and `and (`, and for calling a variable (`$callback()`).
- `unreachable-code` no longer flags the body of a returned closure or anything else in a multi-line `return`, and no longer flags code after a braceless `if ($x) return;`.
- `undefined-variable` no longer flags reassignments, variables compared with `==` or `===`, array keys on write (`$a[$key] = 1`), reused `foreach` variables, `global` and `static` variables, class properties, or variables first created by `$a[] = 1`. Comparisons also stop counting as assignments, so `undefined-variable` now reports a comparison with a variable that really is undefined.
- `undefined-variable` no longer flags variables that a call may write to, such as `$matches` in `preg_match()`. Functions are checked against their signatures where those are known. Method, constructor and unresolved function calls, and builtins without a full signature, get the benefit of the doubt, so an undefined variable passed straight to one of those isn't reported.
- `unused-variable` no longer flags parameters with default values, class properties, `self::$name` writes, or variables only read in a double-quoted string (`"{$name}"`), used as an array key, compared, or captured by `use (...)` after an arrow function.
- Renaming a local variable now also updates its reassignments and its uses inside double-quoted strings.
- **Variable types in hover and inlay hints** now work in more places: closure and arrow function parameters and locals (using `@param` tags in a docblock above the closure), code in traits and enums, casts such as `(int) $x`, and method chains over several lines, which used to show the type of the first line. A variable whose type can't be worked out now hovers as `mixed` instead of showing nothing.
- Variable types no longer come from a same-named variable in the method above, a parameter of the enclosing method when inside a closure, or a same-named property. `$this` inside a trait no longer resolves to the class declared before it.
- Hover keeps a variable's whole type, such as `?string` rather than `string`, including when a method's return type allows `null`.
- Inlay hints now show scalar types when the value comes from a call (`$n = count($a)`), since only literals and casts make them obvious, and no longer appear on parameters with default values.

## [0.9.0] - 2026-10-30

### Added

- `tuskPhpLsp.createObject` command: given a directory URI, an object kind (`class`, `trait`, `interface`, or `enum`), and a name, returns a `WorkspaceEdit` that creates the PHP file and inserts a namespace-aware skeleton. Namespace is derived from PSR-4 autoload mappings; omitted when the path falls outside any mapped root. Returns nothing when the name isn't a valid PHP identifier or the file already exists.
- `tuskPhpLsp.copyReference` command: given a URI and cursor position, resolves the symbol under the cursor and returns labeled reference strings — FQN, short name, `Class::method()`, and `Class::method(Type $param): ReturnType`; `Class::$property` for properties and `Class::CONSTANT` for constants and enum cases — so the editor can present a pick-list for the user to copy.
- **Copy reference, copy/move namespace as code actions**: `tuskPhpLsp.copyNamespace` and `tuskPhpLsp.moveToNamespace` are surfaced as standard LSP code actions (`source` / `refactor.move` kinds), making them accessible in Zed, Neovim, and other editors that expose the code-action menu but not custom command palettes. *Copy reference…* (`refactor`) only appears for clients that list `tuskPhpLsp.copyReference` in the new `clientCommands` initialization option, since copying the result to the clipboard is up to the client.
- **Undefined variable** diagnostic: warns on reads of unbound variables; skips superglobals, `$this`, parameters, foreach/catch/closure bindings, and scopes using `extract()`, `include`, or `eval`.
- **Unused variable** diagnostic: hints on local assignments whose value is never read; ignores underscore-prefixed names (`$_`, `$_foo`) and by-reference bindings.
- **Argument count mismatch** diagnostic: warns when a call passes too many arguments to a project function or static method with a fixed parameter list; skips variadic, spread, and named-argument calls.
- Per-rule severity in `.tusk-php.json`: `diagnosticRules` now accepts `"off"` / `"hint"` / `"info"` / `"warning"` / `"error"` in addition to the existing `true`/`false` toggle.
- `textDocument/selectionRange`: hierarchical selection expansion from word → brackets → method → class → document.
- `textDocument/semanticTokens/full`: semantic highlighting for keywords, strings, numbers, comments, variables, declarations, and property names.
- Match expression parsing: `ParseResult.Matches` captures every `match` structure in a file, including nested matches; recovers gracefully from malformed arms.
- Call-site model: `ParseResult.Calls` records every function, method, static, and constructor call with per-argument names, values, and spread flags (capped at 5 000/file).
- First-class callable detection: `strlen(...)` / `$obj->method(...)` syntax sets `CallSite.IsFirstClassCallable`, distinguishing it from regular and variadic calls.
- **Symfony translation intelligence**: completion and go-to-definition for translation keys across YAML, PHP array, and XLIFF files; understands `$translator->trans()`, `t()`, `new TranslatableMessage()`, and domain-argument forms.
- **Symfony `#[Autowire]` support**: constructor injection now resolves `service:`, `param:`, and `env:` attribute forms.
- **Symfony console command discovery**: indexes `#[AsCommand]` attributes and legacy `protected static $defaultName` declarations.
- `symfony_routes` and `symfony_route_to_controller` MCP tools, plus `tusk://symfony/routes` resource — Symfony workspaces only. Routes are indexed at workspace build time for instant responses.
- `php_container_bindings` MCP tool: lists all resolved DI container bindings (`abstract`, `concrete`, `singleton`, `source`, `alias`, `tags`, `definitionUri`, `range`). Optional `class` input also returns the class's resolved constructor injection (`paramName`, `typeHint`, `resolvedConcrete`, `isSingleton`, plus Symfony `autowireKind`/`autowireValue`). Always registered — returns empty `bindings` on non-framework projects.
- `laravel_model_schema` now includes a `relations[]` list, each entry with `method`, `kind` (e.g. `HasMany`, `BelongsTo`), resolved target FQN (`target`), and `cardinality` (`one`/`many`).
- `contractVersion: 1` field in `php_project_summary` so agents can detect breaking interface changes.
- **Built-in MCP server**: run `tusk-php mcp` to host the MCP tools for AI agents (Claude Code, Codex, and other stdio MCP clients) over stdio; `tusk-php mcp dump` writes a static context pack. No separate binary to install — it's the same `tusk-php` binary the editor already uses.
- **Cross-server benchmark harness** (`make bench`): runs the same editor questions — hover, definition, references, completion, symbols, signature help, diagnostics — against tusk-php, Intelephense, Phpactor, php-lsp and PHPantom at identical caret positions, and reports start-up time, memory, per-feature latency and how many answers were correct. Servers are described in `bench/servers.json` and cases in `bench/suite/*.json`, so extending either is JSON rather than code; `make bench-install` fetches the other servers into a gitignored `bench/.tools/`.
- `deps tree`, `deps usage`, and `deps unused` CLI subcommands for visualising Composer dependency trees, measuring symbol usage, and finding unused packages.
- `--parse` flag: dumps a PHP file's parsed AST as JSON from the command line.
- `introspect` subcommand: dumps parsed state (symbols, diagnostics, active profile) for a file; also available in editors as `tuskPhpLsp.debugDocument`.
- `phpVersion` and `extensions` in `.tusk-php.json` now slot into the builtin profile chain between Composer constraints and local PHP detection.
- Four-step PHP version resolution: Composer platform → `.tusk-php.json` → local `php` binary → bundled default; the winning source is logged on startup.
- `db.allowDataSampling` and `db.connectTimeoutMs` configuration options for database introspection.
- PHP 8.1 stubs: `Fiber`, `array_is_list`, `enum_exists`, `UnitEnum`/`BackedEnum`, Reflection enums/fibers, `AllowDynamicProperties`, `ReturnTypeWillChange`, `CURLStringFile`.
- PHP 8.2 stubs: `ini_parse_quantity`, `SensitiveParameter`/`SensitiveParameterValue`.
- PHP 8.3 stubs: `Override` attribute.
- WASM (`wasip1`) build target for browser-based editors such as vscode.dev.
- **AI agents guide** (`docs/ai-agents.md`): setup instructions for running `tusk-php mcp` in Claude Code, Codex, Cursor and other stdio MCP clients, with a summary of the available tools and resources.

### Changed

- `tusk-php mcp` now introduces itself to MCP clients as `tusk-php` instead of the old `tusk-mcp` name.
- Unified Composer `installedPackage` struct now includes `Version`; removed the `installedPackageWithVersion` duplicate.

### Fixed

- The server no longer freezes on a `match` expression with a stray `)` or `]` in an arm, as can happen while typing one.
- `workspace/executeCommand` capability now lists all three handled commands (`tuskPhpLsp.namespaceForPath`, `tuskPhpLsp.copyNamespace`, `tuskPhpLsp.moveToNamespace`); previously only the first was advertised.
- `laravel_*` MCP tools are now gated on Laravel framework detection and no longer appear in non-Laravel workspaces.
- Variadic parameters (`...$args`) are now correctly detected via the triple-dot token sequence.
- `ClassNode.EndLine` is now exposed in the compatibility layer.

## [0.8.0] - 2026-06-01

### Added

- Hover cards for `composer.json` `require` and `require-dev` entries: hovering a package name shows its installed version (read from `composer.lock`), version constraint, PHP requirement with a compatibility glyph against the workspace PHP version, license, description, and a link to the package's Packagist or VCS page.
- Go-to-Definition on a `composer.json` package name can open the package's Packagist (or configured repository) page in the browser, gated by the new `composer.openOnDefinition` setting and the editor's `window/showDocument` support.
- New settings `tuskPhpLsp.composer.hover.enable` (default on) and `tuskPhpLsp.composer.openOnDefinition` (default off) in the VS Code extension.
- VS Code extension now activates on workspaces containing a `composer.json` and routes JSON files matching that pattern to the server.
- Zed extension now registers JSON as a handled language so `composer.json` files are routed to the server for hover and definition.
- Attribute completion now surfaces indexed attribute classes (user-defined and from vendor) that match the typed prefix, with automatic `use`-import edits inserted alongside the existing built-in attribute suggestions.

### Fixed

- PHP 8 attributes with constructor arguments — such as `#[Char("name", nullable: true)]` — no longer generate spurious "unknown function" diagnostics for the call inside the attribute.
- Named arguments inside attribute constructors — such as `nullable: true` — no longer generate spurious "unknown attribute" diagnostics.

## [0.7.0] - 2026-05-28

### Added

- Generated PHP built-in symbol registry covering functions, classes, interfaces, and traits from PHP's internal runtime tables, preventing known PHP symbols from being reported as unknown when no richer stub exists yet.
- Embedded, versioned PHP stub layers for core symbols and extensions, including initial PHP 7.4/8.0 core stubs and JSON extension stubs through PHP 8.3.
- Project-aware builtin profile selection from Composer platform data, Composer `require.php` constraints, required `ext-*` packages, local PHP detection, and the bundled default profile.
- `builtin-unavailable` diagnostics for known PHP builtins that exist outside the project's selected PHP version or enabled extension set, such as PHP 8 functions used in a PHP 7.4 project.
- Local PHP version detection via the `php` binary, with configurable timeout handling and fallback to bundled stubs when detection is unavailable.
- PHP manual URL generation for builtin methods, properties, class constants, predefined constants, classes, interfaces, traits, enums, and functions.
- Configurable PHP manual locale through `php_manual_locale`.
- Optional browser opening for builtin definitions through `php_manual_open_on_definition`, using the LSP `window/showDocument` capability when supported by the client.
- VS Code settings for PHP manual locale and builtin definition opening.
- A builtin-generation helper script for registry, stub, and availability metadata generation.
- Additional resolver support for `class-string<T>` constructor binding, typed access-chain walking, `foreach` element propagation, and Reflection generic metadata.
- Unknown PHP attribute diagnostics, including detection for builtin attributes that are unavailable in the selected PHP profile.

### Changed

- Builtin registration now uses profile-aware stub layering, with generated fallback symbols loaded before richer curated stubs.
- LSP initialization now resolves and logs the effective PHP builtin profile from Composer, local PHP, or bundled defaults.
- Unknown-symbol diagnostics use a softer mode while indexing settles, reducing noisy false positives during startup.
- Hover and definition resolution now use declaring classes for inherited builtin members so PHP manual links point to the correct manual page.
- Hover resolution now prefers PHPDoc `@return` types over broad native placeholder types such as `object`, `mixed`, and `array` when that gives a more useful result.
- Documentation was refreshed for project architecture, builtin navigation, and the new PHP manual settings.
- Moved the Go module and repository references from `github.com/open-southeners/tusk-php` to `github.com/Tusk-PHP/lsp`, including internal Go imports, release/download URLs, and contributor documentation.

### Fixed

- Unknown-function and unknown-class diagnostics no longer flag broad PHP builtins such as `array_reverse`, `ReflectionObject`, `ReflectionMethod`, and `InvalidArgumentException`.
- Builtin symbols are filtered from workspace navigation results where jumping to generated or embedded builtin declarations would be unhelpful.
- Class properties are excluded from standalone name lookup, preventing property symbols from resolving as unrelated bare identifiers.
- Manual links for receiver-class and inherited-member hovers now strip leading namespace separators and resolve against the correct owner class.
- Hover and resolver behavior for unknown typed class variables, property access chains, and string-based variable assignments is more accurate.
- Attribute syntax and local PHP detection timeout handling are covered by the resolved diagnostic follow-ups.

## [0.6.1] - 2026-05-25

### Added

- Composer-generated `vendor/composer/autoload_files.php` parsing so global helper files registered through Composer are indexed even when package metadata is unavailable or incomplete.
- Additional PHP built-in function symbols for common array, regex, type-checking, constant, class, object, and function-introspection helpers.

### Fixed

- Hover cards no longer resolve words inside PHP string literals as unrelated symbols, preventing array declaration keys such as `'environment' => [...]` from showing constant or method hover cards.
- Unknown-function diagnostics no longer flag PHP built-ins such as `get_class()` or indexed Laravel/Composer global helpers such as `class_basename()`.

## [0.6.0] - 2026-05-22

### Added

- Configurable inlay hints for inferred variable types, `foreach` key/value types, closure and method return types, and call-site parameter names.
- New LSP navigation surfaces for type definitions, implementations, document highlights, folding ranges, and richer workspace symbol results.
- Native unknown-symbol diagnostics for unresolved classes, functions, and members.
- Code actions for unused imports, import organization, and unknown-class import quick fixes.
- Refactor code actions for implementing missing methods, generating constructors, generating getters and setters, extracting variables, and conservatively inlining local variables.
- Structured PHPDoc support for conditional type rendering and local type-alias resolution across completion, hover, and type resolution.
- Laravel route, view, and translation metadata indexes with completion and definition support for framework string references.
- Symfony route-name intelligence for attribute routes plus YAML/XML route config and imported route resources.
- Symfony container discovery for YAML and PHP service configuration, including fluent PHP `set()` and `alias()` definitions.

### Changed

- Renamed the server binary, bundled editor binaries, logs, and project config references from `php-lsp` to `tusk-php`.
- Cursor analysis and scoped variable tracking now model position-aware namespaces/imports, free functions, closures, arrow functions, grouped imports, closure captures, destructuring, and assignment metadata used by rename and refactor actions.
- Container service-ID navigation now distinguishes service-definition locations from type-definition targets.
- Inlay hints render shorter type names when fully qualified names can be reduced for display.

### Fixed

- Generic and docblock type resolution for annotated parameters and union generic carriers, including recursion isolation for concurrent resolver work.
- Parser recovery errors are preserved and published as syntax diagnostics with stable locations for malformed input.
- Enum interface implementations now participate in implementation lookup.
- Variable rename preparation no longer advertises unsupported unscoped variable renames.
- Position-aware cursor scope and import resolution prevent later namespace/import state from leaking into earlier source regions.

## [0.5.0] - 2026-05-17

### Added

- PHP 8.3–8.5 syntax support in the parser: property hooks (`{ get => ...; set => ...; }`), the `|>` pipe operator, asymmetric visibility (`public private(set)`), and dynamic class constant fetch (`Class::{$name}`) — constructs that were previously silently dropped or mis-parsed.
- Hover cards for properties now show PHP 8.4 property hooks and asymmetric `(set)` visibility.

### Changed

- Faster workspace-wide symbol prefix search via binary lookup, improving completion responsiveness on large projects.
- Find references now reuses indexed in-memory source instead of re-reading every file from disk on each request.
- Large files are indexed off the JSON-RPC message loop, keeping the server responsive while big documents are processed.

### Fixed

- Fatal crash (stack overflow) in hover and completion when resolving self-referential or mutually-referential variable assignments.
- Excessive work and apparent hangs in the variable-type chain resolver on certain repeated assignment patterns.
- Property hook bodies leaking their local variables (`$value`, `$this`) as spurious class properties in completion and document symbols.
- Completion returning blank entries when completing namespace segments in `use` statements.
- Completion results being returned in a non-deterministic order between identical requests.
- Signature help highlighting a parameter position past the end of the parameter list when more arguments than parameters were typed.

## [0.4.0] - 2026-03-25

### Added

- Laravel Facade support: static calls like `Cache::get()` now resolve through the container via `getFacadeAccessor()`, providing completions and hover cards from the concrete class's methods.
- Facade concrete method completion: methods not declared in `@method static` annotations but present on the resolved concrete class are surfaced as static completions.
- Nested trait member resolution at all depth levels (trait using another trait using another trait, etc.), propagating methods and properties through the full chain.

### Fixed

- `@method static` docblock annotations not setting `IsStatic` on virtual members, causing them to be filtered out of `ClassName::` completions.
- Trait `use` declarations inside other traits being silently discarded by the parser, preventing nested trait members from appearing in completions and hover.
- `traitMap` entries not being cleaned up on file re-index, causing stale trait associations to persist until process restart.
- Windows workspace path containing percent-encoded characters (`%3A` for `:`) not being decoded, causing the LSP server to index zero files.
- All `file://` URI-to-path conversions now use proper URL decoding and Windows drive letter handling via the shared `symbols.URIToPath` helper.

## [0.3.2] - 2026-03-24

### Fixed

- VSCode extension shipping without bundled LSP binaries due to release CI not copying platform binaries into the extension's `bin/` directory before packaging the `.vsix`.

## [0.3.1] - 2026-03-24

### Fixed

- VSCode extension failing to start the bundled LSP binary on Windows (`spawn tusk-php ENOENT`) by appending `.exe` to the PATH fallback command.
- VSCode extension failing to start the bundled LSP binary on Linux/macOS remote environments by ensuring execute permissions after `.vsix` extraction.
- Zed extension build failure caused by `unicode-segmentation` 1.13.0 breaking `heck` 0.4.1 (`UnicodeWords` made private); pinned to 1.12.0.

## [0.3.0] - 2026-03-24

### Added

- Generic-aware type resolution for template-annotated classes and methods, preserving concrete type parameters through completions, hover, and chained calls.
- Generic-aware support for Laravel Eloquent builders and collections, including concrete model propagation for methods such as `query()`, `where()`, `get()`, `first()`, `find()`, and related collection helpers.
- Array literal and array-shape inference in resolved expression types, improving type preservation for inferred variables and collection payloads.
- Local `scripts/set-version.sh` utility to update repository version strings for releases.

### Changed

- Release automation now targets plain semantic version tags without the previous release prefix.
- Expanded Laravel and Symfony end-to-end fixtures and coverage around generics, array shapes, and framework controller flows.

### Fixed

- Incorrect generic propagation across LSP completions, hover results, and shared resolver paths.
- Variable type inference for generic return values, including Laravel helpers such as `Arr::first()` and `Collection::first()`.
- Symfony test fixture compatibility issues that were causing CI instability.

## [0.2.0] - 2026-03-23

### Added

- Multi-source model attribute discovery for Eloquent and Doctrine ORM models.
- Virtual property injection from `@property`/`@method` docblock tags on classes.
- IDE helper file support (`_ide_helper_models.php`, `_ide_helper.php`) with member merging into existing indexed classes.
- Eloquent relation discovery from method return types and `$this->hasMany()`/`belongsTo()`/etc. calls.
- Eloquent accessor/mutator detection (both legacy `getNameAttribute()` and modern `Attribute` cast).
- Eloquent virtual static methods (`where`, `find`, `first`, `with`, `orderBy`, etc.) mimicking `__callStatic` forwarding.
- Doctrine ORM entity support: `#[Column]`, `#[Id]`, `#[GeneratedValue]`, `#[OneToMany]`, `#[ManyToOne]`, `#[OneToOne]`, `#[ManyToMany]` PHP 8 attribute parsing.
- Doctrine repository class detection and association to entities via `#[Entity(repositoryClass: ...)]`.
- Database schema introspection for MySQL, PostgreSQL, and SQLite via `.env` credentials with in-memory caching.
- Laravel migration file parsing to extract column definitions (`$table->string()`, `$table->foreignId()`, `$table->timestamps()`, etc.).
- Builder string argument completion for column names inside `where('`, `orderBy('`, `select('`, `pluck('`, `groupBy('`, etc.
- Builder string argument completion for relation names inside `with('`, `has('`, `whereHas('`, `load('`, `withCount('`, etc.
- Array argument support for Builder completions (`select(['`, `get(['`, `with(['`, etc.).
- Strict DB-only column filtering for `get()` (only suggests columns from IDE helper, database introspection, or migrations).
- `.env` file parser for reading database connection settings.
- `database` configuration option in LSP initialization settings to enable/disable DB introspection.
- Built-in diagnostics engine with standalone `internal/checks/` package reusable by CLI tools and CI pipelines.
- Unused import detection with word-boundary scanning, aliased imports, `use function`/`use const`, and PHP 8 attributes.
- Unused private method and property detection (excludes magic methods).
- Unreachable code detection after `return`, `throw`, `exit`/`die`, `continue`, `break`.
- Redundant union member detection (duplicates, `?Type|null`, supertype subsumption for `mixed`, `object`, `iterable`).
- Redundant nullsafe `?->` operator detection on non-nullable types.
- Unknown column validation in Builder string arguments (`where`, `orderBy`, `select`, `get`, etc.).
- Unknown relation validation in Builder string arguments (`with`, `has`, `whereHas`, `load`, etc.).
- Aggregate relation method second-arg validation (`withSum('relation', 'column')` checks column on the related model).
- `DiagnosticTag` support: unused code greyed out (`Unnecessary`), deprecated functions struck through (`Deprecated`).
- `diagnosticRules` configuration in `.tusk-php.json` to enable/disable individual diagnostic rules.
- Multi-line method chain resolution for hover, go-to-definition, and completions.
- `resolve.JoinChainLines()` helper to join continuation lines starting with `->`, `::`, or `?->`.
- Comprehensive chain resolution test suite covering single-line and multi-line Eloquent method chains.

### Fixed

- Hover cards and go-to-definition landing on wrong vendor symbols when method chains span multiple lines.
- Completion provider now resolves multi-line chains correctly instead of falling through to global completions.

## [0.1.0] - 2026-03-21

### Added

- Initial `Tusk PHP LSP` release with a Go-based PHP language server binary.
- VS Code extension packaging for the bundled LSP client.
- Zed extension packaging for the WebAssembly-based extension.
- Cross-platform release artifacts for Linux, macOS, and Windows.


