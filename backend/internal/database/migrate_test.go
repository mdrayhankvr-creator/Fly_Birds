package database

import "testing"

func TestSQLStatementsKeepFunctionBodies(t *testing.T) {
	body := "-- comment;\nCREATE FUNCTION f() RETURNS void AS $$ BEGIN PERFORM 1; PERFORM 2; END; $$ LANGUAGE plpgsql; INSERT INTO t VALUES ('a;b');"
	statements := sqlStatements(body)
	if len(statements) != 2 {
		t.Fatalf("got %d statements: %#v", len(statements), statements)
	}
	if statements[0] == "" || statements[1] != "INSERT INTO t VALUES ('a;b')" {
		t.Fatalf("unexpected split: %#v", statements)
	}
}
