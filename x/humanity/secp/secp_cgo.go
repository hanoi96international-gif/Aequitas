//go:build cgo

package secp

/*
#cgo CFLAGS: -I./libsecp256k1
#cgo CFLAGS: -I./libsecp256k1/src/

#ifndef NDEBUG
#  define NDEBUG
#endif

#include "umbenennen.h"
#include "./libsecp256k1/src/secp256k1.c"
#include "./libsecp256k1/src/modules/recovery/main_impl.h"
#include "./libsecp256k1/src/precomputed_ecmult.c"
#include "./libsecp256k1/src/precomputed_ecmult_gen.c"

// Wie secp256k1_ext_ecdsa_recover in go-ethereum (crypto/secp256k1/ext.h).
static int aeq_wiederherstellen(
	const secp256k1_context* ctx,
	unsigned char *pubkey_out,
	const unsigned char *sigdata,
	const unsigned char *msgdata
) {
	secp256k1_ecdsa_recoverable_signature sig;
	secp256k1_pubkey pubkey;
	size_t outputlen = 65;

	if (!secp256k1_ecdsa_recoverable_signature_parse_compact(ctx, &sig, sigdata, (int)sigdata[64])) {
		return 0;
	}
	if (!secp256k1_ecdsa_recover(ctx, &pubkey, &sig, msgdata)) {
		return 0;
	}
	return secp256k1_ec_pubkey_serialize(ctx, pubkey_out, &outputlen, &pubkey, SECP256K1_EC_UNCOMPRESSED);
}

static secp256k1_context* aeq_kontext(void) {
	return secp256k1_context_create(SECP256K1_CONTEXT_NONE);
}
*/
import "C"

import "unsafe"

// Nur lesend benutzt; libsecp256k1 erlaubt das aus beliebig vielen Threads.
var kontext = C.aeq_kontext()

// Wiederherstellen liefert den unkomprimierten oeffentlichen Schluessel
// (65 Byte, 0x04 ...) zur Nachricht (32 Byte) und Signatur [R || S || V]
// mit V in 0..3. Verhalten wie go-ethereum crypto.Ecrecover.
func Wiederherstellen(nachricht, sig []byte) ([]byte, error) {
	if err := pruefeEingabe(nachricht, sig); err != nil {
		return nil, err
	}
	var (
		pubkey  = make([]byte, 65)
		sigdata = (*C.uchar)(unsafe.Pointer(&sig[0]))
		msgdata = (*C.uchar)(unsafe.Pointer(&nachricht[0]))
	)
	if C.aeq_wiederherstellen(kontext, (*C.uchar)(unsafe.Pointer(&pubkey[0])), sigdata, msgdata) == 0 {
		return nil, ErrNichtHergestellt
	}
	return pubkey, nil
}

// Schnell: ob diese Kopie benutzt wird (sonst go-ethereum, siehe secp_nocgo.go).
const Schnell = true
