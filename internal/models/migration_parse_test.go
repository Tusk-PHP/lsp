package models

import (
	"sort"
	"strings"
	"testing"
)

func columnNames(cols map[string]*MigrationColumn) string {
	names := make([]string, 0, len(cols))
	for n := range cols {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func TestParseMigrationFileReadsOnlyUp(t *testing.T) {
	tables := map[string]map[string]*MigrationColumn{}
	parseMigrationFile(`<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::create('users', function (Blueprint $table) {
            $table->id();
            $table->string('name');
        });
        Schema::table('users', function (Blueprint $t) {
            $t->integer('votes_available')->default(0)->after('name');
        });
    }

    public function down(): void
    {
        Schema::table('users', function (Blueprint $table) {
            $table->dropColumn('votes_available');
        });
        Schema::dropIfExists('users');
    }
};
`, tables)
	if got := columnNames(tables["users"]); got != "id,name,votes_available" {
		t.Errorf("users: %s", got)
	}
	if d := tables["users"]["votes_available"].Default; d != "0" {
		t.Errorf("default: %q", d)
	}
}

func TestParseMigrationFileBlueprintMethods(t *testing.T) {
	tables := map[string]map[string]*MigrationColumn{}
	parseMigrationFile(`<?php
class CreateReports extends Migration {
    public function up()
    {
        Schema::create("reports", static function (Blueprint $table) {
            $table->bigIncrements("id");
            $table->morphs('reportable');
            $table->nullableUuidMorphs('target');
            $table->foreignIdFor(\App\Models\User::class);
            $table->foreignIdFor(Team::class, 'owner_team_id')->nullable();
            $table->string("title")->nullable();
            $table->string('old_name');
            $table->renameColumn('old_name', 'new_name');
            $table->string('gone');
            $table->dropColumn(['gone']);
            // $table->string('commented');
            $table->string("interpolated_{$x}");
            $table->softDeletes();
            $table->timestamps();
        });
        Schema::connection('audit')->create('audits', function (Blueprint $table) {
            $table->id();
        });
        Schema::create('temp', fn ($t) => null);
        Schema::rename('temp', 'temporary');
    }
}
`, tables)
	want := "created_at,deleted_at,id,new_name,owner_team_id,reportable_id,reportable_type,target_id,target_type,title,updated_at,user_id"
	if got := columnNames(tables["reports"]); got != want {
		t.Errorf("reports:\n got %s\nwant %s", got, want)
	}
	r := tables["reports"]
	if !r["title"].Nullable || !r["owner_team_id"].Nullable || !r["target_id"].Nullable || r["reportable_id"].Nullable {
		t.Error("nullable flags are wrong")
	}
	if r["target_id"].Type != "string" || r["reportable_id"].Type != "int" {
		t.Errorf("morph id types: %s %s", r["target_id"].Type, r["reportable_id"].Type)
	}
	if got := columnNames(tables["audits"]); got != "id" {
		t.Errorf("audits: %s", got)
	}
	if _, ok := tables["temporary"]; !ok {
		t.Error("rename lost the table")
	}
	if _, ok := tables["temp"]; ok {
		t.Error("rename kept the old table")
	}
}

func TestParseMigrationFileWithoutUpSkipsDown(t *testing.T) {
	tables := map[string]map[string]*MigrationColumn{}
	parseMigrationFile(`<?php
Schema::create('flags', function (Blueprint $table) {
    $table->boolean('on');
});
function down() {
    Schema::drop('flags');
}
`, tables)
	if got := columnNames(tables["flags"]); got != "on" {
		t.Errorf("flags: %q", got)
	}
}
