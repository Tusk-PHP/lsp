package models

import (
	"strconv"
	"strings"
	"testing"
)

func BenchmarkParseMigrationFile(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\n\nuse Illuminate\\Database\\Migrations\\Migration;\nuse Illuminate\\Database\\Schema\\Blueprint;\n\nreturn new class extends Migration {\n    public function up(): void\n    {\n")
	for i := range 20 {
		n := strconv.Itoa(i)
		src.WriteString("        Schema::create('table_" + n + "', function (Blueprint $table) {\n            $table->id();\n")
		for c := range 15 {
			src.WriteString("            $table->string('column_" + strconv.Itoa(c) + "')->nullable()->default('x'); // a column\n")
		}
		src.WriteString("            $table->foreignIdFor(User::class);\n            $table->timestamps();\n        });\n")
	}
	src.WriteString("    }\n\n    public function down(): void\n    {\n        Schema::dropIfExists('table_0');\n    }\n};\n")
	source := src.String()
	b.ReportAllocs()
	for b.Loop() {
		tables := map[string]map[string]*MigrationColumn{}
		parseMigrationFile(source, tables)
		if len(tables) != 20 {
			b.Fatalf("got %d tables, want 20", len(tables))
		}
	}
}
