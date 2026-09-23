package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnnahmeVerweigert(t *testing.T) {
	knoten := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/health/combined" {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(body))
		}))
	}
	lesend := knoten(`{"annahme_tor":{"nimmt_an":false,"rolle":"nur_lesend"}}`)
	defer lesend.Close()
	annehmend := knoten(`{"annahme_tor":{"nimmt_an":true}}`)
	defer annehmend.Close()
	alt := knoten(`{"chain":{}}`)
	defer alt.Close()

	hc := &http.Client{}
	if _, v := annahmeVerweigert([]string{annehmend.URL + "/rpc"}, hc); v {
		t.Fatal("ein annehmender Knoten darf nicht abbrechen")
	}
	if _, v := annahmeVerweigert([]string{alt.URL + "/rpc"}, hc); v {
		t.Fatal("ein Knoten ohne das Feld (aelterer Stand) darf nicht abbrechen")
	}
	if _, v := annahmeVerweigert([]string{"http://127.0.0.1:1/rpc"}, hc); v {
		t.Fatal("ein unerreichbarer Knoten darf nicht als Verweigerung gelten")
	}
	basis, v := annahmeVerweigert([]string{annehmend.URL + "/rpc", lesend.URL + "/rpc"}, hc)
	if !v || basis != lesend.URL {
		t.Fatalf("der nur lesende Knoten muss erkannt werden, bekam %q %v", basis, v)
	}
}

func TestAussortierenNichtVorDerBefuellung(t *testing.T) {
	if aussortierenVorgesehen("fund", "1000000000000000") {
		t.Fatal("vor der Befuellung darf nicht aussortiert werden -- es traefe genau die Konten, die befuellt werden sollen")
	}
	if aussortierenVorgesehen("fund,warmup,run", "1000000000000000") {
		t.Fatal("auch nicht, wenn nach der Befuellung gleich gemessen wird")
	}
	if !aussortierenVorgesehen("warmup,run", "1000000000000000") {
		t.Fatal("vor einer reinen Messung muss aussortiert werden -- leere Absender reissen ihre Buendel mit")
	}
	if aussortierenVorgesehen("warmup,run", "0") || aussortierenVorgesehen("warmup,run", "") {
		t.Fatal("ohne Mindestguthaben wird nicht aussortiert")
	}
}
