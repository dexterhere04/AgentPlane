# API Key Design Evaluation

## Overview
The API key design is a critical part of the gateway architecture. The selected approach must satisfy the following requirements:

* Securely generate API keys
* Associate each key with a user or organization
* Authenticate incoming requests efficiently
* Avoid storing plaintext secrets
* Support key revocation and rotation
* Enable auditing and usage tracking
* Keep the rest of the gateway independent from authentication details
* Scale to support enterprise workloads

---

# Evaluated Approaches

## Approach A — Key ID + Secret

### API Key Format

```text
ap_live_<key_id>_<secret>
```

Example:

```text
ap_live_7F92KD82_a81f93c82d72e91
```

### Authentication Flow

1. The client sends the API key.
2. AgentPlane extracts the `key_id`.
3. The database performs an indexed lookup using the `key_id`.
4. The provided secret is hashed.
5. The computed hash is compared with the stored hash using a constant-time comparison.
6. If the hashes match, the request is authenticated and the associated user is resolved.

### Advantages

#### Efficient Database Lookup

The `key_id` acts as a unique identifier that can be indexed in the database. Instead of searching using the entire API key, AgentPlane can immediately locate the corresponding record and verify only the secret.

This provides:

* Fast indexed lookups
* Lower authentication latency
* Better scalability as the number of API keys increases

#### Secure Secret Storage

Only the secret portion of the API key is stored as a cryptographic hash.

As a result:

* Plaintext API keys are never stored.
* A database compromise does not expose usable credentials.
* Authentication follows well-established password verification practices.

#### Better Key Rotation

Users can own multiple active API keys simultaneously.

Example:

```text
Production
ap_live_A12BCD34_xxxxxxxxx

Development
ap_live_F98JKL12_xxxxxxxxx
```

Each key can be revoked, regenerated, or assigned different permissions independently without affecting the others.

#### Better Auditability

The `key_id` is not sensitive and can safely appear in:

* Authentication logs
* Monitoring dashboards
* Audit trails
* Support tools
* Usage reports

Example:

```text
Authenticated key: 7F92KD82
```

This allows operators to identify which credential was used without exposing any secret information.

#### Better Operational Visibility

The structured format immediately communicates useful information.

```text
ap_live_7F92KD82_a81f93c82d72e91
```

Where:

* `ap` identifies AgentPlane.
* `live` identifies the environment.
* `key_id` identifies the credential.
* `secret` is the confidential authentication component.

This structure simplifies debugging, monitoring, and incident response.

#### Enterprise Scalability

As AgentPlane grows, this format naturally supports additional enterprise capabilities such as:

* Multiple organizations
* Service accounts
* Fine-grained permissions
* Usage analytics
* Key expiration
* Rate limiting per key
* Provider-specific access
* Model-specific permissions

These features can be implemented by attaching metadata to the `key_id` without changing the authentication mechanism.

### Limitations

* API keys are slightly longer.
* Authentication requires parsing the key before verification.
* Key generation is marginally more complex than generating a single random string.

These trade-offs are minor compared to the long-term operational benefits.

---

# Approach B — Opaque API Key

## API Key Format

```text
ap_sk_live_xxxxxxxxxxxxxxxxx
```

Example:

```text
ap_sk_live_3f84a7d91d0d6a82ef...
```

### Authentication Flow

1. The client sends the API key.
2. The gateway hashes the entire key.
3. The gateway performs a lookup using the hash.
4. If a matching record exists, the request is authenticated.

### Advantages

#### Simpler Design

The API key consists of a single randomly generated secret, making implementation straightforward.

#### Easy Developer Experience

Developers only need to manage one string, resulting in a simple and familiar API key format.

#### Strong Security

When generated with sufficient randomness and stored as a cryptographic hash, opaque API keys provide strong security against credential disclosure.

### Limitations

#### Reduced Operational Visibility

Since the entire API key is confidential, there is no safe identifier that can be displayed in logs, dashboards, or audit reports.

Administrators often need to create additional internal identifiers to determine which credential was used.

#### More Difficult Key Management

Without a separate identifier:

* Troubleshooting authentication issues becomes harder.
* Audit logs are less informative.
* Support tooling becomes more complex.
* Administrators have fewer ways to identify individual credentials.

#### Limited Extensibility

As enterprise requirements evolve, features such as organization management, permission systems, and administrative tooling become harder to build because the API key itself contains no non-sensitive identifier.

---

# Comparison

| Criteria                    | Approach A (Key ID + Secret) | Approach B (Opaque Key) |
| --------------------------- | ---------------------------- | ----------------------- |
| Secure secret storage       | Yes                          | Yes                     |
| Plaintext keys never stored | Yes                          | Yes                     |
| Indexed lookup              | Yes                          | No                      |
| Authentication efficiency   | Excellent                    | Good                    |
| Key rotation                | Excellent                    | Good                    |
| Multiple active keys        | Yes                          | Yes                     |
| Safe logging                | Yes                          | No                      |
| Auditability                | Excellent                    | Limited                 |
| Operational visibility      | Excellent                    | Limited                 |
| Enterprise scalability      | Excellent                    | Moderate                |
| Implementation simplicity   | Moderate                     | Excellent               |

---

# Final Decision

After evaluating both approaches against AgentPlane's architectural requirements, **Approach A (`ap_live_<key_id>_<secret>`) is selected as the API key format.**

Although both approaches securely authenticate requests and avoid storing plaintext credentials, the Key ID + Secret design provides significant advantages for a production-grade AI Gateway.

Separating the non-sensitive `key_id` from the secret allows AgentPlane to perform efficient indexed lookups, reducing authentication overhead while maintaining strong security. It also provides a safe identifier that can be used in logs, dashboards, monitoring systems, and audit trails without exposing sensitive information.

In addition, the structured format supports multiple active keys per user, seamless key rotation, fine-grained permission management, usage tracking, and future enterprise capabilities without requiring changes to the authentication workflow.

The opaque API key approach is a valid design and is suitable for smaller systems where implementation simplicity is the primary goal. However, its lack of a non-sensitive identifier reduces operational visibility and makes future expansion more difficult.

For these reasons, AgentPlane adopts the following API key format:

```text
ap_live_<key_id>_<secret>
```

Example:

```text
ap_live_7F92KD82_a81f93c82d72e91
```

This design provides the best balance of security, scalability, operational efficiency, maintainability, observability, and developer experience, making it the most suitable choice for AgentPlane.
