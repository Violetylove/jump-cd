package pathutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Violetylove/jump-cd/internal/pathutil"
)

func TestNormalizeExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, err := pathutil.Normalize("~/code")
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	want := filepath.Join(home, "code")
	if !pathutil.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeRejectsNewline(t *testing.T) {
	if _, err := pathutil.Normalize("/tmp/a" + string(rune(10)) + "b"); err == nil {
		t.Fatal("expected error for newline path")
	}
}

func TestSegments(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"posix", "/home/u/code/jump-cd", []string{"home", "u", "code", "jump-cd"}},
		{"trailing slash", "/a/b/", []string{"a", "b"}},
		{"root", "/", nil},
		{"dot segments dropped", "/a/./b/../c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathutil.Segments(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v want %v", got, tt.want)
				}
			}
		})
	}
}

func TestSegmentsDropsVolume(t *testing.T) {
	p := filepath.Join("C:"+string(filepath.Separator), "Users", "w", "code")
	segs := pathutil.Segments(p)

	if pathutil.IsWindows() {
		for _, s := range segs {
			if len(s) == 2 && s[1] == ':' {
				t.Fatalf("volume segment leaked: %v", segs)
			}
		}
	}
	if got := pathutil.Base(p); got != "code" {
		t.Fatalf("Base = %q", got)
	}
}

func TestIsUnder(t *testing.T) {
	sep := string(filepath.Separator)
	root := filepath.Join(sep, "a")
	sub := filepath.Join(root, "b")
	deep := filepath.Join(sub, "c")
	other := filepath.Join(sep, "ab")

	tests := []struct {
		name          string
		parent, child string
		want          bool
	}{
		{"equal", sub, sub, true},
		{"direct child", sub, deep, true},
		{"grandparent", root, deep, true},
		{"not under", sub, other, false},
		{"prefix trap", root, other, false},
		{"reversed", deep, root, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathutil.IsUnder(tt.parent, tt.child); got != tt.want {
				t.Fatalf("IsUnder(%q,%q) = %v want %v", tt.parent, tt.child, got, tt.want)
			}
		})
	}
}

func TestSameParent(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "code")
	a := filepath.Join(base, "jump-cd")
	b := filepath.Join(base, "web")
	if !pathutil.SameParent(a, b) {
		t.Fatalf("expected %q and %q to share a parent", a, b)
	}
	if pathutil.SameParent(a, a) {
		t.Fatal("a path is not its own sibling")
	}
}

func TestEqualCaseSensitivity(t *testing.T) {
	a := filepath.Join(string(filepath.Separator), "Code", "App")
	b := filepath.Join(string(filepath.Separator), "code", "app")
	if got, want := pathutil.Equal(a, b), pathutil.IsWindows(); got != want {
		t.Fatalf("Equal = %v, want %v (windows=%v)", got, want, pathutil.IsWindows())
	}
}

// git-bash 下 cd 出来的 $PWD 是 MSYS 形式，原生程序必须能看懂。
func TestMSYSPathTranslation(t *testing.T) {
	if !pathutil.IsWindows() {
		t.Skip("仅 Windows 有意义")
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"盘符挂载", "/c/Users/x", filepath.Join("C:"+string(filepath.Separator), "Users", "x")},
		{"cygdrive", "/cygdrive/d/code", filepath.Join("D:"+string(filepath.Separator), "code")},
		{"裸盘符", "/c", "C:" + string(filepath.Separator)},
		{"tmp 伪路径", "/tmp/scratch", filepath.Join(os.TempDir(), "scratch")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pathutil.Normalize(tt.in)
			if err != nil {
				t.Fatalf("Normalize(%q): %v", tt.in, err)
			}
			if !pathutil.Equal(got, tt.want) {
				t.Fatalf("Normalize(%q) = %q want %q", tt.in, got, tt.want)
			}
		})
	}

	// 猜一个盘符只会把用户带到不存在的目录，所以要明确报错。
	if _, err := pathutil.Normalize("/usr/local/bin"); err == nil {
		t.Fatal("MSYS 伪路径 /usr/... 应当报错")
	}
}

func TestValidateOutput(t *testing.T) {
	if err := pathutil.ValidateOutput("/ok"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if err := pathutil.ValidateOutput("bad" + string(rune(10)) + "path"); err == nil {
		t.Fatal("expected newline to be rejected")
	}
	if err := pathutil.ValidateOutput(""); err == nil {
		t.Fatal("expected empty to be rejected")
	}
}
