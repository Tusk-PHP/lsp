package symbols

type builtinNamedAvailability struct {
	Name      string
	Since     string
	Extension string
	Removed   string
}

var generatedBuiltinFunctionAvailabilityByName = builtinAvailabilityByNameOf(generatedBuiltinFunctionAvailability)

func builtinAvailabilityByNameOf(rows []builtinNamedAvailability) map[string]BuiltinAvailability {
	out := make(map[string]BuiltinAvailability, len(rows))
	for _, r := range rows {
		out[r.Name] = BuiltinAvailability{Since: r.Since, Extension: r.Extension, Removed: r.Removed}
	}
	return out
}
