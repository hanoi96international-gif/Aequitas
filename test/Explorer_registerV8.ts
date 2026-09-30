import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";
import { Wallet, getBytes, toBeHex } from "ethers";

// Die Website (explorer.js, vom Knoten ausgeliefert) muss fuer V8 genau das
// unterschreiben, was jeder Knoten prueft. Der Block zwischen
// V8-REGISTER-BEGIN/END wird hier mit dem AUSGELIEFERTEN ethers
// (assets/vendor/ethers.min.js) ausgefuehrt und gegen den gemeinsamen
// Pruefvektor von Go (TestV8_GoldenVektorMitDerApp) und App geprueft.

const here = path.dirname(fileURLToPath(import.meta.url));
const assets = path.join(here, "..", "x", "humanity", "keeper", "assets");

function ladeBlock() {
  const js = readFileSync(path.join(assets, "explorer.js"), "utf8");
  const start = js.indexOf("// V8-REGISTER-BEGIN");
  const ende = js.indexOf("// V8-REGISTER-END");
  assert.ok(start > 0 && ende > start, "V8-Block in explorer.js fehlt");
  const v7 = /const V7_CONTRACT = '(0x[0-9a-fA-F]{40})'/.exec(js);
  assert.ok(v7, "V7_CONTRACT fehlt");
  const ctx: any = { module: undefined, exports: undefined, define: undefined, TextEncoder, TextDecoder, crypto: globalThis.crypto, console };
  vm.createContext(ctx);
  // Wie im Browser: ethers sucht sein globales Objekt ueber self/window.
  vm.runInContext("globalThis.self = globalThis; globalThis.window = globalThis;", ctx);
  vm.runInContext(readFileSync(path.join(assets, "vendor", "ethers.min.js"), "utf8"), ctx);
  assert.ok(ctx.ethers?.TypedDataEncoder, "ausgeliefertes ethers ohne TypedDataEncoder");
  vm.runInContext(
    `const V7_CONTRACT = '${v7[1]}';\n${js.slice(start, ende)}\n` +
      `globalThis.V8 = { v8RegisterVertrag, v8TypedData, v8Signatur, v8Unterschreiben, V8_FRIST_S };`,
    ctx,
  );
  return { V8: ctx.V8, ethers: ctx.ethers };
}

const { V8, ethers } = ladeBlock();
const KENNUNG = "aequitas-1926-1790000000";
const ALICE = new Wallet("0x" + "11".repeat(32));
const MALLORY = new Wallet("0x" + "22".repeat(32));
const SIGNALE = ["1001", "2001"];
const FRIST = 1_790_000_600;
const DIGEST = "0x0cfd835d1262f048ab9e759f064e7d6ebfb747599e40a606e5cd84dc13cc1804";
const SIG =
  "0x9d5387fee26b5ec00237ff732f68821b31744e07c81b8b815f048c1c9ad8965d4032040a16fbf1ef18bd9c24bbdab9c76df4c03fa8637ce003f18a4dc4d05cd31c";
const N = 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141n;

// Eine Wallet wie MetaMask: nimmt das JSON von eth_signTypedData_v4 und
// unterschreibt es mit ihrem Schluessel.
function wallet(w: Wallet, verbiegen: (s: string) => string = (s) => s) {
  const calls: any[] = [];
  return {
    calls,
    request: async ({ method, params }: any) => {
      calls.push({ method, params });
      assert.equal(method, "eth_signTypedData_v4");
      const p = JSON.parse(params[1]);
      const { EIP712Domain: _, ...types } = p.types;
      return verbiegen(await w.signTypedData(p.domain, types, p.message));
    },
  };
}

describe("explorer.js: V8-Registrierung (EIP-712)", () => {
  it("Pruefvektor: Digest und Signatur byte-gleich zu Go und App", async () => {
    const td = V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST);
    assert.equal(ethers.TypedDataEncoder.hash(td.domain, td.types, td.message), DIGEST);
    const w = wallet(ALICE);
    assert.equal(await V8.v8Unterschreiben(w, KENNUNG, ALICE.address.toLowerCase(), SIGNALE, FRIST), SIG);
    const p = JSON.parse(w.calls[0].params[1]);
    assert.deepEqual(p.types.EIP712Domain.map((f: any) => f.name), ["name", "version", "chainId", "verifyingContract", "salt"]);
    assert.equal(p.message.nonce, "0");
  });

  it("gleicht v 0/1 an", async () => {
    const w = wallet(ALICE, (s) => s.slice(0, -2) + (parseInt(s.slice(-2), 16) - 27).toString(16).padStart(2, "0"));
    assert.equal(await V8.v8Unterschreiben(w, KENNUNG, ALICE.address, SIGNALE, FRIST), SIG);
  });

  it("fremde Wallet hinter der Adresse: abgelehnt", async () => {
    await assert.rejects(V8.v8Unterschreiben(wallet(MALLORY), KENNUNG, ALICE.address, SIGNALE, FRIST), /not from this wallet/);
  });

  it("Unterschrift aus anderem Netz oder fuer andere Signale: abgelehnt", async () => {
    const alt = await ALICE.signTypedData(
      ...(() => { const t = V8.v8TypedData("aequitas-1926-1700000000", ALICE.address, SIGNALE, FRIST); return [t.domain, t.types, t.message] as const; })(),
    );
    const td = V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST);
    assert.throws(() => V8.v8Signatur(td, alt), /not from this wallet/);
    const vertauscht = V8.v8TypedData(KENNUNG, ALICE.address, ["2001", "1001"], FRIST);
    assert.throws(() => V8.v8Signatur(vertauscht, SIG), /not from this wallet/);
  });

  it("hohes s, falsche Laenge, falsches v: abgelehnt", () => {
    const td = V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, FRIST);
    const b = getBytes(SIG);
    const s = BigInt("0x" + SIG.slice(66, 130));
    const formbar = SIG.slice(0, 66) + toBeHex(N - s, 32).slice(2) + (b[64] === 27 ? "1c" : "1b");
    assert.throws(() => V8.v8Signatur(td, formbar), /high s/);
    assert.throws(() => V8.v8Signatur(td, SIG.slice(0, -2)), /65 bytes/);
    assert.throws(() => V8.v8Signatur(td, SIG.slice(0, -2) + "1d"), /invalid v/);
  });

  it("ungueltige Eingaben: nicht unterschreiben", () => {
    const r = "21888242871839275222246405745257275088548364400416034343698204186575808495617";
    for (const [k, sig] of [
      ["aequitas-1-1790000000", SIGNALE],
      [undefined, SIGNALE],
      [KENNUNG, ["0", "1"]],
      [KENNUNG, ["1", r]],
      [KENNUNG, ["0x10", "2"]],
      [KENNUNG, ["1"]],
    ] as const) {
      assert.throws(() => V8.v8TypedData(k, ALICE.address, sig, FRIST));
    }
    assert.throws(() => V8.v8TypedData(KENNUNG, "0x1234", SIGNALE, FRIST));
    assert.throws(() => V8.v8TypedData(KENNUNG, ALICE.address, SIGNALE, 0));
  });

  it("Vertragsversion: fehlend = v7, unbekannt = null; Frist unter einem Tag", () => {
    assert.equal(V8.v8RegisterVertrag({}), "v7");
    assert.equal(V8.v8RegisterVertrag({ register_vertrag: "v8" }), "v8");
    assert.equal(V8.v8RegisterVertrag({ register_vertrag: "v9" }), null);
    assert.equal(V8.v8RegisterVertrag(null), null);
    assert.ok(V8.V8_FRIST_S > 15 * 60 && V8.V8_FRIST_S < 24 * 60 * 60);
  });
});
