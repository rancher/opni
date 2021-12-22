/*
Package bootstrap contains logic for securely adding new clusters
to the gateway using bootstrap tokens.

The bootstrap process is as follows:

1. The server generates a self-signed keypair, and a bootstrap token.
2. The client is given the bootstrap token and the hash of the server's
	 expected root CA certificate (by a human). It sends a request to the
	 server's /bootstrap endpoint with no Authentication header. The client
	 cannot yet trust the server's self-signed certificate, so it does not send
	 any other data in the request.
3. The server responds with its root CA certificate, several JWS messages
	 with detached payloads (one for each active bootstrap token) and an
	 ephemeral x25519 public key.
4. The client compares the hash of the RawSubjectPublicKeyInfo of the server's
	 root CA certificate to the expected value. It also ensures that the leaf
	 of the as-yet-unverified TLS cert chain presented by the server was signed
	 by the expected root CA cert. If the hash matches and the certs are correct,
	 the client can now trust the server.
5. The client finds the JWS with the matching bootstrap token ID, fills in
	 the detached payload (the bootstrap token), encrypts it with the server's
	 leaf TLS certificate, and sends it back to the server along with the
	 client's own unique identifier it wishes to use (typically the client's
	 kube-system namespace resource UID)
6. The server decrypts the JWS with its leaf private key, and verifies the
	 reconstructed JWS. The server can now trust the client.
7. Both the client and server use their ephemeral keypair and their peer's
	 public key to generate a shared secret. Then, this secret is passed through
	 a KDF to create two static ed25519 keys. One key is used by the client to
	 sign outgoing messages, and by the server to verify incoming messages.
	 The other key is used for the cortex tenant ID.
*/
package bootstrap
