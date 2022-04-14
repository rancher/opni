/*
Package bootstrap contains logic for securely adding new clusters
to the gateway using bootstrap tokens.

The bootstrap process is as follows:

1. The server generates a self-signed keypair, and a bootstrap token.
2. The client is given the bootstrap token and one or more fingerprints of
	 public keys in the server's certificate chain ("pinned" public keys).
	 It sends a request to the server's /bootstrap/join endpoint with no
	 Authentication header. The client cannot yet trust the server's self-signed
	 certificate, so it does not send any other data in the request.
4. During the TLS handshake, the client computes the fingerprints of the public
	 keys in the server's offered certificates, and compares them to its
	 pinned fingerprints. If any of the fingerprints match, and the server's
	 certificate chain is valid (i.e. each certificate is signed by the next
	 certificate in the chain), the client trusts the server and completes the
	 TLS handshake.
3. The server responds with several JWS messages with detached payloads
	 (one for each active bootstrap token).
5. The client finds the JWS with the matching bootstrap token ID, fills in
	 the detached payload (the bootstrap token), and sends it back to the server's
	 /bootstrap/join endpoint along with the client's own unique identifier it
	 wishes to use (typically the client's kube-system namespace resource UID)
	 and an ephemeral x25519 public key.
6. The server verifies the reconstructed JWS. If it is correct, the server can
   now trust the client. The server responds with its own ephemeral x25519
	 public key.
7. Both the client and server use their ephemeral keypair and their peer's
	 public key to generate a shared secret. Then, this secret is passed through
	 a KDF to create two static ed25519 keys. One is used to generate and verify
	 MACs for client->server messages, and the other is used to generate and
	 verify MACs for server->client messages.
*/
package bootstrap
