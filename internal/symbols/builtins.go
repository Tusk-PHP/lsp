package symbols

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/stubs"
)

type builtinFunction struct {
	Name   string
	Params []ParamInfo
	Ret    string
	Doc    string
}

var builtinFunctions = []builtinFunction{
	{"isset", []ParamInfo{{Name: "$var", Type: "mixed"}}, "bool", "Check if variable is set"},
	{"empty", []ParamInfo{{Name: "$var", Type: "mixed"}}, "bool", "Check if variable is empty"},
	{"class_basename", []ParamInfo{{Name: "$class", Type: "string|object"}}, "string", "Get the class basename of the given object or class"},
}

var builtinClasses = []struct{ Name, Doc string }{
	{"stdClass", "Generic empty class"},
	{"Exception", "Base class for all exceptions"},
	{"DateTime", "Representation of date and time"},
	{"DateTimeImmutable", "Immutable date and time"},
	{"Fiber", "Full-stack interruptible functions (PHP 8.1+)"},
	{"WeakMap", "Maps objects as keys to arbitrary values"},
	{"Generator", "Generator objects returned from generators"},
	{"Closure", "Class used to represent anonymous functions"},
	{"ArrayObject", "Allows objects to work as arrays"},
}

// BuiltinProfile selects the PHP version and extension layers used for
// built-in symbol registration.
type BuiltinProfile struct {
	PHPVersion string
	Extensions []string
}

// BuiltinAvailability records the version and optional extension constraint
// for a PHP built-in symbol.
type BuiltinAvailability struct {
	Since     string
	Extension string
	Removed   string
}

func (a BuiltinAvailability) RemovedBy(version string) bool {
	return a.Removed != "" && compareBuiltinVersions(a.Removed, version) <= 0
}

func (a BuiltinAvailability) InVersion(version string) bool {
	return builtinInVersion(a.Since, a.Removed, version)
}

var builtinAvailabilityByName = map[string]BuiltinAvailability{
	"APC_LIST_ACTIVE":   {Extension: "apcu"},
	"APC_LIST_DELETED":  {Extension: "apcu"},
	"APC_ITER_TYPE":     {Extension: "apcu"},
	"APC_ITER_KEY":      {Extension: "apcu"},
	"APC_ITER_VALUE":    {Extension: "apcu"},
	"APC_ITER_NUM_HITS": {Extension: "apcu"},
	"APC_ITER_MTIME":    {Extension: "apcu"},
	"APC_ITER_CTIME":    {Extension: "apcu"},
	"APC_ITER_DTIME":    {Extension: "apcu"},
	"APC_ITER_ATIME":    {Extension: "apcu"},
	"APC_ITER_REFCOUNT": {Extension: "apcu"},
	"APC_ITER_MEM_SIZE": {Extension: "apcu"},
	"APC_ITER_TTL":      {Extension: "apcu"},
	"APC_ITER_NONE":     {Extension: "apcu"},
	"APC_ITER_ALL":      {Extension: "apcu"},

	// Optional PECL APIs; keep these names available to builtin-unavailable
	// without loading any extension stub source into the symbol index.
	"SSH2_DEFAULT_TERMINAL":       {Extension: "ssh2"},
	"SSH2_DEFAULT_TERM_HEIGHT":    {Extension: "ssh2"},
	"SSH2_DEFAULT_TERM_UNIT":      {Extension: "ssh2"},
	"SSH2_DEFAULT_TERM_WIDTH":     {Extension: "ssh2"},
	"SSH2_FINGERPRINT_HEX":        {Extension: "ssh2"},
	"SSH2_FINGERPRINT_MD5":        {Extension: "ssh2"},
	"SSH2_FINGERPRINT_RAW":        {Extension: "ssh2"},
	"SSH2_FINGERPRINT_SHA1":       {Extension: "ssh2"},
	"SSH2_FINGERPRINT_SHA256":     {Extension: "ssh2"},
	"SSH2_HOSTKEY_TYPE_DSS":       {Extension: "ssh2"},
	"SSH2_HOSTKEY_TYPE_ECDSA_256": {Extension: "ssh2"},
	"SSH2_HOSTKEY_TYPE_ECDSA_384": {Extension: "ssh2"},
	"SSH2_HOSTKEY_TYPE_ECDSA_521": {Extension: "ssh2"},
	"SSH2_HOSTKEY_TYPE_ED25519":   {Extension: "ssh2"},
	"SSH2_HOSTKEY_TYPE_RSA":       {Extension: "ssh2"},
	"SSH2_HOSTKEY_TYPE_UNKNOWN":   {Extension: "ssh2"},
	"SSH2_POLLERR":                {Extension: "ssh2"},
	"SSH2_POLLEXT":                {Extension: "ssh2"},
	"SSH2_POLLHUP":                {Extension: "ssh2"},
	"SSH2_POLLIN":                 {Extension: "ssh2"},
	"SSH2_POLLNVAL":               {Extension: "ssh2"},
	"SSH2_POLLOUT":                {Extension: "ssh2"},
	"SSH2_POLL_CHANNEL_CLOSED":    {Extension: "ssh2"},
	"SSH2_POLL_LISTENER_CLOSED":   {Extension: "ssh2"},
	"SSH2_POLL_SESSION_CLOSED":    {Extension: "ssh2"},
	"SSH2_STREAM_STDERR":          {Extension: "ssh2"},
	"SSH2_STREAM_STDIO":           {Extension: "ssh2"},
	"SSH2_TERM_UNIT_CHARS":        {Extension: "ssh2"},
	"SSH2_TERM_UNIT_PIXELS":       {Extension: "ssh2"},

	// PHP 7.3 constants
	"JSON_THROW_ON_ERROR": {Since: "7.3"},

	// PHP 7.4 constants
	"T_FN":                  {Since: "7.4"},
	"ARRAY_FILTER_USE_BOTH": {Since: "7.4"},

	// PHP 8.0 constants
	"FILTER_VALIDATE_BOOL":   {Since: "8.0"},
	"T_NAME_QUALIFIED":       {Since: "8.0"},
	"T_NAME_FULLY_QUALIFIED": {Since: "8.0"},
	"T_NAME_RELATIVE":        {Since: "8.0"},

	// PHP 8.1 constants
	"MYSQLI_REFRESH_REPLICA": {Since: "8.1"},
	"CURLOPT_DOH_URL":        {Since: "8.1"},

	// PHP 8.3 constants
	"SEEK_HOLE": {Since: "8.3"},
	"SEEK_DATA": {Since: "8.3"},
}

// RegisterBuiltins populates the index with PHP built-in symbols.
func (idx *Index) RegisterBuiltins() {
	idx.RegisterBuiltinsForProfile(DefaultBuiltinProfile())
}

// DefaultBuiltinProfile returns the broadest bundled built-in profile.
func DefaultBuiltinProfile() BuiltinProfile {
	profile := stubs.DefaultProfile()
	return BuiltinProfile{PHPVersion: profile.PHPVersion, Extensions: profile.Extensions}
}

// RegisterBuiltinsForProfile populates the index with built-in symbols
// available for a PHP version/extension profile.
func (idx *Index) RegisterBuiltinsForProfile(profile BuiltinProfile) {
	profile = normalizeBuiltinProfile(profile)
	idx.registerBuiltinRegistry(profile)
	idx.RegisterBuiltinStubs(profile)
}

func (idx *Index) registerBuiltinRegistry(profile BuiltinProfile) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.registerBuiltinRegistryLocked(profile)
}

func (idx *Index) registerBuiltinRegistryLocked(profile BuiltinProfile) {
	for i := range generatedBuiltinFunctionSignatures {
		sig := &generatedBuiltinFunctionSignatures[i]
		if !builtinAvailable(sig.Name, profile) {
			continue
		}
		idx.addBuiltinFunctionLocked(sig.Name, sig.Params, sig.ReturnType, sig.Docs.docFor(profile.PHPVersion))
	}

	for _, fn := range builtinFunctions {
		if !builtinAvailable(fn.Name, profile) {
			continue
		}
		idx.addBuiltinFunctionLocked(fn.Name, fn.Params, fn.Ret, fn.Doc)
	}

	for _, name := range generatedBuiltinFunctionNames {
		if idx.builtinLocked(name) != nil {
			continue
		}
		if !builtinAvailable(name, profile) {
			continue
		}
		idx.addBuiltinFunctionLocked(name, nil, "mixed", "PHP built-in function")
	}

	for _, c := range generatedBuiltinClassLikes {
		idx.addBuiltinClassLikeWithMembersLocked(c, profile)
	}

	for _, cls := range builtinClasses {
		if sym := idx.builtinLocked(cls.Name); sym != nil {
			if sym.DocComment == "" {
				sym.DocComment = cls.Doc
			}
			continue
		}
		if !builtinAvailable(cls.Name, profile) {
			continue
		}
		idx.addBuiltinClassLikeLocked(cls.Name, cls.Doc)
	}

	for _, name := range generatedBuiltinClassLikeNames {
		if idx.builtinLocked(name) != nil {
			continue
		}
		if !builtinAvailable(name, profile) {
			continue
		}
		idx.addBuiltinClassLikeLocked(name, "PHP built-in class-like symbol")
	}
}

// RegisterBuiltinStubs indexes embedded PHP stub declarations as built-in
// symbols. Stubs intentionally run after the generated registry so richer
// signatures, class kinds, methods, and constants replace generic fallbacks.
func (idx *Index) RegisterBuiltinStubs(profile BuiltinProfile) {
	entries, err := stubs.BuiltinPHPForProfile(stubs.Profile{PHPVersion: profile.PHPVersion, Extensions: profile.Extensions})
	if err != nil {
		return
	}
	for _, entry := range entries {
		idx.IndexFileWithSource("builtin://"+entry.Path, entry.Content, SourceBuiltin)
	}
}

func normalizeBuiltinProfile(profile BuiltinProfile) BuiltinProfile {
	if strings.TrimSpace(profile.PHPVersion) == "" {
		profile.PHPVersion = DefaultBuiltinProfile().PHPVersion
	}
	seen := make(map[string]struct{}, len(profile.Extensions))
	extensions := make([]string, 0, len(profile.Extensions))
	for _, ext := range profile.Extensions {
		ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), "ext-")
		if ext == "" {
			continue
		}
		if _, ok := seen[ext]; ok {
			continue
		}
		seen[ext] = struct{}{}
		extensions = append(extensions, ext)
	}
	sort.Strings(extensions)
	profile.Extensions = extensions
	return profile
}

func builtinAvailable(name string, profile BuiltinProfile) bool {
	availability, ok := LookupAvailability(name)
	if !ok {
		return true
	}
	if !availability.InVersion(profile.PHPVersion) {
		return false
	}
	if availability.Extension != "" {
		for _, ext := range profile.Extensions {
			if ext == availability.Extension {
				return true
			}
		}
		return false
	}
	return true
}

// LookupAvailability reports whether name is a known PHP built-in subject
// to a version or extension constraint. Class-likes and functions come from
// the generated tables (builtin_classes_generated.go,
// builtin_functions_generated.go); constants from builtinAvailabilityByName in
// this file.
func LookupAvailability(name string) (BuiltinAvailability, bool) {
	if a, ok := generatedBuiltinClassAvailability[name]; ok {
		return a, true
	}
	if a, ok := generatedBuiltinFunctionAvailabilityByName[name]; ok {
		return a, true
	}
	if a, ok := builtinAvailabilityByName[name]; ok {
		return a, true
	}
	return BuiltinAvailability{}, false
}

func compareBuiltinVersions(left string, right string) int {
	leftMajor, leftMinor := parseBuiltinMajorMinor(left)
	rightMajor, rightMinor := parseBuiltinMajorMinor(right)
	if leftMajor != rightMajor {
		if leftMajor < rightMajor {
			return -1
		}
		return 1
	}
	if leftMinor != rightMinor {
		if leftMinor < rightMinor {
			return -1
		}
		return 1
	}
	return 0
}

func parseBuiltinMajorMinor(version string) (int, int) {
	parts := strings.Split(version, ".")
	major := 0
	minor := 0
	if len(parts) > 0 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	return major, minor
}

func (idx *Index) addBuiltinFunctionLocked(name string, params []ParamInfo, ret string, doc string) {
	idx.storeBuiltinLocked(&Symbol{Name: name, FQN: name, Kind: KindFunction, Source: SourceBuiltin, URI: builtinRegistryURI, ReturnType: ret, DocComment: doc, Params: params})
}

func (idx *Index) addBuiltinClassLikeLocked(name string, doc string) {
	idx.storeBuiltinLocked(&Symbol{Name: name, FQN: name, Kind: KindClass, Source: SourceBuiltin, URI: builtinRegistryURI, DocComment: doc})
}

func (idx *Index) storeBuiltinLocked(sym *Symbol) {
	prev := idx.symbols[sym.FQN]
	switch {
	case prev == nil:
		idx.storeSymbolLocked(sym, pendingRemovals{})
		idx.fileSymbols[sym.URI] = append(idx.fileSymbols[sym.URI], sym)
	case prev.URI == sym.URI:
		idx.symbols[sym.FQN] = sym
		idx.fileSymbols[sym.URI] = withSymbolReplaced(idx.fileSymbols[sym.URI], prev, sym)
	default:
		idx.shadowBuiltinLocked(sym)
	}
}

func (idx *Index) shadowBuiltinLocked(sym *Symbol) {
	defs := idx.shadowed[sym.FQN]
	if i := slices.IndexFunc(defs, func(def *Symbol) bool { return def.Source == SourceBuiltin }); i >= 0 {
		if defs[i].URI == sym.URI {
			idx.fileSymbols[sym.URI] = withSymbolReplaced(idx.fileSymbols[sym.URI], defs[i], sym)
		} else {
			idx.fileSymbols[sym.URI] = append(idx.fileSymbols[sym.URI], sym)
		}
		defs[i] = sym
		return
	}
	if idx.shadowed == nil {
		idx.shadowed = make(map[string][]*Symbol)
	}
	idx.shadowed[sym.FQN] = append([]*Symbol{sym}, defs...)
	idx.fileSymbols[sym.URI] = append(idx.fileSymbols[sym.URI], sym)
}
