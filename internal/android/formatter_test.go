package android

import "testing"

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		name, source, target string
		valid                bool
	}{
		{"plain", "Hello", "Hallo", true},
		{"indexed reorder", "Hello %1$s, you have %2$d messages", "%2$d Nachrichten für %1$s", true},
		{"implicit to explicit", "%s has %d", "%2$d für %1$s", true},
		{"date and reuse", "%1$tF at %<tR", "%1$tR am %1$tF", true},
		{"reuse", "%s %<s", "%1$s %1$s", true},
		{"mixed indexing", "%2$s %s %<s %s", "%1$s %2$s %1$s %2$s", true},
		{"reuse after percent", "%s %% %<s", "%1$s %1$s %%", true},
		{"literals", "%s 100%% %n", "100%% %1$s %n", true},
		{"flag order", "%1$+,12.2f", "%1$,+12.2f", true},
		{"general conversions", "%b %h %s %c %d %o %x %e %f %g %a", "%b %h %s %c %d %o %x %e %f %g %a", true},
		{"format changed", "%1$.2f", "%1$.3f", false},
		{"flag changed", "%1$08d", "%1$8d", false},
		{"wrong type", "%s %d", "%s %s", false},
		{"wrong order", "%s %d", "%d %s", false},
		{"repeated argument lost", "%1$s %1$s", "%1$s", false},
		{"repeated formatting swapped", "%1$s %1$10s", "%1$s %1$s", false},
		{"date suffix changed", "%1$tF", "%1$tD", false},
		{"date invalid", "%tJ", "%tJ", false},
		{"date precision", "%.2tF", "%.2tF", false},
		{"Foundation object", "%@", "%@", false},
		{"Foundation long", "%ld", "%ld", false},
		{"Foundation long long", "%lld", "%lld", false},
		{"dynamic width", "%*s", "%*s", false},
		{"stray percent", "Discount 30%", "Rabatt 30%", false},
		{"unknown conversion", "%q", "%q", false},
		{"uppercase invalid F", "%F", "%F", false},
		{"empty precision", "%.s", "%.s", false},
		{"zero index", "%0$s", "%0$s", false},
		{"overflow index", "%2147483648$s", "%2147483648$s", false},
		{"overflow width", "%2147483648s", "%2147483648s", false},
		{"overflow precision", "%.2147483648s", "%.2147483648s", false},
		{"duplicate flags", "%--5s", "%--5s", false},
		{"missing width", "%-s", "%-s", false},
		{"invalid general flag", "%+s", "%+s", false},
		{"invalid integer precision", "%.2d", "%.2d", false},
		{"invalid integer flag", "%#d", "%#d", false},
		{"invalid exponent grouping", "%,e", "%,e", false},
		{"invalid general floating flag", "%#g", "%#g", false},
		{"incompatible flags", "%+ d", "%+ d", false},
		{"incompatible padding", "%-05d", "%-05d", false},
		{"newline width", "%5n", "%5n", false},
		{"percent precision", "%.2%", "%.2%", false},
		{"no prior argument", "%<s", "%<s", false},
		{"invalid target format", "%s", "%lld", false},
		{"literal added", "Hello", "Hallo %n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFormat(tt.source, tt.target)
			if (err == nil) != tt.valid {
				t.Fatalf("ValidateFormat(%q,%q) = %v; valid=%v", tt.source, tt.target, err, tt.valid)
			}
		})
	}
}
