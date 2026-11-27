package laravel

import "github.com/Tusk-PHP/lsp/internal/symbols"

func TemplateFallbacks() map[string]symbols.TemplateFallback {
	return map[string]symbols.TemplateFallback{
		EloquentBuilder: {
			Params: []string{"TModel"},
			Methods: map[string]string{
				// Query execution — return model or collection
				"get":            "Illuminate\\Database\\Eloquent\\Collection<int, TModel>",
				"first":          "?TModel",
				"firstOrFail":    "TModel",
				"find":           "?TModel",
				"findOrFail":     "TModel",
				"findMany":       "Illuminate\\Database\\Eloquent\\Collection<int, TModel>",
				"create":         "TModel",
				"firstOrNew":     "TModel",
				"firstOrCreate":  "TModel",
				"updateOrCreate": "TModel",
				"sole":           "TModel",
				// Builder methods — return static (preserves Builder<TModel>)
				"where": "static", "whereIn": "static", "whereNotIn": "static",
				"whereNull": "static", "whereNotNull": "static",
				"whereBetween": "static", "whereNotBetween": "static",
				"whereDate": "static", "whereMonth": "static", "whereDay": "static",
				"whereYear": "static", "whereTime": "static", "whereColumn": "static",
				"orderBy": "static", "orderByDesc": "static",
				"latest": "static", "oldest": "static",
				"limit": "static", "take": "static", "skip": "static", "offset": "static",
				"groupBy": "static", "having": "static",
				"select": "static", "addSelect": "static",
				"distinct": "static",
				"with":     "static", "without": "static",
				"has": "static", "doesntHave": "static",
				"whereHas": "static", "whereDoesntHave": "static",
				"withCount": "static",
				"join":      "static", "leftJoin": "static",
				"withTrashed": "static", "onlyTrashed": "static",
				"when": "static", "unless": "static",
				"tap": "static",
			},
		},
		"Illuminate\\Database\\Eloquent\\Collection": {
			Params: []string{"TKey", "TModel"},
			Methods: map[string]string{
				"first":   "?TModel",
				"last":    "?TModel",
				"find":    "?TModel",
				"map":     "Illuminate\\Support\\Collection<TKey, mixed>",
				"pluck":   "Illuminate\\Support\\Collection",
				"all":     "array",
				"toArray": "array",
				"count":   "int",
				"filter":  "static",
				"reject":  "static",
				"unique":  "static",
				"values":  "static",
				"sortBy":  "static",
				"each":    "static",
			},
		},
		"Illuminate\\Support\\Collection": {
			Params: []string{"TKey", "TValue"},
			Methods: map[string]string{
				"first":   "?TValue",
				"last":    "?TValue",
				"map":     "static",
				"filter":  "static",
				"reject":  "static",
				"count":   "int",
				"all":     "array",
				"toArray": "array",
				"values":  "static",
				"keys":    "static",
				"each":    "static",
			},
		},
	}
}
