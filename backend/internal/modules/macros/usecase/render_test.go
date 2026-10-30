package usecase

import "testing"

func TestRender(t *testing.T) {
	vars := map[string]string{"contact.first_name": "Ayşe", "agent.name": "Elif Şahin", "conversation.number": "42"}
	got := Render("Merhaba {{contact.first_name}}, ben {{ agent.name }}. #{{conversation.number}} {{unknown}}!", vars)
	if got != "Merhaba Ayşe, ben Elif Şahin. #42 !" {
		t.Fatalf("Render = %q", got)
	}
	if firstName("  Can  Öztürk ") != "Can" || firstName("") != "" {
		t.Fatal("firstName")
	}
}

func TestMacroValidation(t *testing.T) {
	bad := []MacroInput{
		{Name: "x"},
		{Name: "x", Actions: []Action{{Type: "explode"}}},
		{Name: "x", Actions: []Action{{Type: ActionStatus, Value: "snoozed"}}},
		{Name: "x", Actions: []Action{{Type: ActionReply}}},
		{Name: "x", Actions: []Action{{Type: ActionAssign, Value: "bob"}}},
	}
	for i, in := range bad {
		if _, err := in.normalize(); err == nil {
			t.Fatalf("case %d should fail", i)
		}
	}
	ok := MacroInput{Name: " İade ", Actions: []Action{{Type: ActionReply, Value: "Tamam"}, {Type: ActionAssign, Value: "me"}, {Type: ActionStatus, Value: "resolved"}}}
	if _, err := ok.normalize(); err != nil || ok.Name != "İade" {
		t.Fatalf("valid macro: %v", err)
	}
}
