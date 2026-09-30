package redact

import "testing"

func TestASecretThatStartsAnotherHidesTheWholeLongerSecretInEitherOrder(t *testing.T) {
	text := "db=hunter2 admin=hunter2-admin-9f"
	for name, values := range map[string][]string{
		"short first": {"hunter2", "hunter2-admin-9f"},
		"long first":  {"hunter2-admin-9f", "hunter2"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Values(text, values, "[secret]"); got != "db=[secret] admin=[secret]" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestOverlappingSecretsThatStartAtDifferentPlacesBecomeOneMark(t *testing.T) {
	// "abcd" と "cdef" は "abcdef" の中で重なる。長さの順に置き換えるだけでは端が残る。
	if got := Values("x abcdef y", []string{"abcd", "cdef"}, "*"); got != "x * y" {
		t.Fatalf("got %q", got)
	}
}

func TestASecretRepeatedOverItselfIsHiddenCompletely(t *testing.T) {
	if got := Values("[aaa]", []string{"aa"}, "*"); got != "[*]" {
		t.Fatalf("got %q", got)
	}
}

func TestAdjacentSecretsBecomeOneMark(t *testing.T) {
	if got := Values("<onetwo>", []string{"one", "two"}, "*"); got != "<*>" {
		t.Fatalf("got %q", got)
	}
}

func TestEmptySecretsAndTextWithoutSecretsAreLeftAlone(t *testing.T) {
	if got := Values("nothing here", []string{"", "absent"}, "*"); got != "nothing here" {
		t.Fatalf("got %q", got)
	}
}

func TestASecretIsNotSearchedBeforeItsStart(t *testing.T) {
	text := []byte("before hunter2 | after hunter2")
	got := Bytes(text, []Secret{{Value: []byte("hunter2"), From: 9}}, "*")
	if string(got) != "before hunter2 | after *" {
		t.Fatalf("got %q", got)
	}
}

func TestTheHeadOfASecretCutAtTheBoundaryIsHidden(t *testing.T) {
	text := []byte("token=top-se")
	got := Bytes(text, []Secret{{Value: []byte("top-secret"), CutAt: len(text)}}, "*")
	if string(got) != "token=*" {
		t.Fatalf("got %q", got)
	}
}

func TestAWordThatMerelyEndsWithTheSecretsFirstLettersIsNotHidden(t *testing.T) {
	// "ready" の末尾 "dy" は "dynamo-secret" の頭と同じだが、単語の途中なので伏せない。
	text := []byte("shell ready")
	got := Bytes(text, []Secret{{Value: []byte("dynamo-secret"), CutAt: len(text)}}, "*")
	if string(got) != "shell ready" {
		t.Fatalf("got %q", got)
	}
}

func TestTheCutHeadAndAWholeSecretBeforeItAreBothHidden(t *testing.T) {
	text := []byte("a=hunter2 b=hunter2-ad")
	got := Bytes(text, []Secret{
		{Value: []byte("hunter2"), CutAt: len(text)},
		{Value: []byte("hunter2-admin-9f"), CutAt: len(text)},
	}, "*")
	if string(got) != "a=* b=*" {
		t.Fatalf("got %q", got)
	}
}

func TestTheInputIsNotModified(t *testing.T) {
	text := []byte("keep hunter2")
	Bytes(text, []Secret{{Value: []byte("hunter2")}}, "*")
	if string(text) != "keep hunter2" {
		t.Fatalf("input changed to %q", text)
	}
}
