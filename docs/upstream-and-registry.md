## 1. Purpose

The gateway originally had one provider (`proxy.NewOpenAIProviderFromEnv()` passed straight into `handlers.Chat`). This work introduces the building blocks that let *any* `proxy.Provider` implementation take part in routing:

```text
                    PROVIDER / UPSTREAM FOUNDATION

                         Application Startup
                                |
                                v
                       +-------------------+
                       | Provider          |
                       | Implementation    |
                       |                   |
                       | OpenAIProvider    |
                       +---------+---------+
                                 |
                                 | wrapped as
                                 v
                       +-------------------+
                       |     Upstream      |
                       +-------------------+
                       | Name              |
                       | Provider          |
                       | Weight            |
                       | Supported Models  |
                       | Timeout           |
                       | Max Retries       |
                       | Enabled/Disabled  |
                       | Circuit Breaker   |
                       +---------+---------+
                                 |
                                 | registered into
                                 v
                       +-------------------+
                       |     Registry      |
                       +-------------------+
                                 |
              +------------------+------------------+
              |                  |                  |
              v                  v                  v
        +-----------+      +-----------+      +-----------+
        |  OpenAI   |      | Anthropic |      |  Google   |
        | Upstream  |      | Upstream  |      | Upstream  |
        +-----------+      +-----------+      +-----------+
              |                  |                  |
              +------------------+------------------+
                                 |
                                 v
                       +-------------------+
                       |      Router       |
                       |    (Next Task)    |
                       +-------------------+
```

The key architectural property: **routing never needs to know the concrete provider type.** Everything downstream of the registry sees only `*Upstream` and the `Provider` interface.

The responsibility of each layer is:

```text
Provider
    |
    | Knows how to communicate with one external backend
    v
Upstream
    |
    | Adds configuration and protection around that provider
    | - weight
    | - supported models
    | - timeout
    | - retries
    | - circuit breaker
    v
Registry
    |
    | Keeps track of all registered upstreams
    | - register
    | - lookup
    | - enable/disable
    | - enumerate
    | - filter candidates
    v
Router
    |
    | Chooses which eligible upstream handles a request
    | (Next task)
    v
Selected Upstream
    |
    v
External Provider API
```

### Provider, Upstream, Registry and Router

| Component  | Simple responsibility                                                                     |
| ---------- | ----------------------------------------------------------------------------------------- |
| `Provider` | Knows how to communicate with one external provider.                                      |
| `Upstream` | Wraps a provider with configuration and protection such as retries and a circuit breaker. |
| `Registry` | Keeps the application's collection of upstreams and finds which ones can handle a model.  |
| `Router`   | Chooses which eligible upstream receives a request.                                       |

The Registry does **not** choose a winner. It provides the Router with the upstreams that are currently eligible.

```text
Request
   |
   v
Registry.Candidates("gpt-4o")
   |
   +---- OpenAI       ✓ enabled
   |
   +---- Anthropic    ✗ model not supported
   |
   +---- Google       ✗ disabled
   |
   v
[OpenAI]
   |
   v
Router chooses upstream
   |
   v
OpenAI Upstream
   |
   v
OpenAIProvider
   |
   v
OpenAI API
```

---

## 3. `Upstream`

An `Upstream` represents **one provider together with the configuration and protection needed to safely use it**.

```text
                         Upstream
                            |
             +--------------+--------------+
             |              |              |
             v              v              v
        Model Check     Enabled?      Circuit Breaker
             |              |              |
             +--------------+--------------+
                            |
                       Available?
                            |
                  +---------+---------+
                  |                   |
                 YES                  NO
                  |                   |
                  v                   v
             Forward Request      Reject / Skip
                  |
                  v
             Provider API
                  |
             +----+----+
             |         |
           Success   Failure
             |         |
             v         v
          Return    Retry if
           data     appropriate
                       |
                       v
                 Circuit Breaker
```

The basic relationship is:

```text
Provider  = how to communicate with the external provider

Upstream  = Provider + configuration + protection

Registry  = collection of Upstreams

Router    = chooses an Upstream
```

For example:

```text
OpenAIProvider
      |
      v
+--------------------------+
| OpenAI Upstream          |
|--------------------------|
| Name: openai             |
| Weight: 70               |
| Models: gpt-*             |
| Timeout: 30s             |
| MaxRetries: 2            |
| Breaker: closed           |
| Enabled: true             |
+--------------------------+
      |
      v
OpenAI API
```

### Circuit breaker flow

The circuit breaker is a safety mechanism that stops sending requests to a provider that is repeatedly failing.

```text
                         +---------+
                         | CLOSED  |
                         | Normal  |
                         +----+----+
                              |
                     enough failures
                              |
                              v
                         +---------+
                         |  OPEN   |
                         | Stop    |
                         | traffic |
                         +----+----+
                              |
                         cooldown
                              |
                              v
                       +-------------+
                       | HALF-OPEN   |
                       | One test    |
                       | request     |
                       +------+------+
                              |
                    +---------+---------+
                    |                   |
                 success              failure
                    |                   |
                    v                   v
                +-------+           +-------+
                |CLOSED |           | OPEN  |
                +-------+           +-------+
```

This means the upstream protects the application without requiring the Router to understand the details of retries, timeouts, or circuit-breaker state.

---

## 5. `Registry`

The Registry is the application's **directory of available upstreams**.

```text
                         Registry
                            |
          +-----------------+-----------------+
          |                 |                 |
          v                 v                 v
     +-----------+     +-----------+     +-----------+
     |  OpenAI   |     | Anthropic |     |  Google   |
     | Upstream  |     | Upstream  |     | Upstream  |
     +-----------+     +-----------+     +-----------+
```

The Registry does not decide which provider should win.

It answers questions such as:

```text
"Do we have an OpenAI upstream?"
        |
        v
     Get("openai")

"What upstreams are registered?"
        |
        v
       List()

"Which upstreams can handle gpt-4o?"
        |
        v
 Candidates("gpt-4o")

"Is OpenAI enabled?"
        |
        v
 SetEnabled("openai", ...)
```

The resulting architecture is:

```text
                    PostgreSQL
                        |
                        | configuration
                        v
                  Application
                        |
                 creates/configures
                        |
                        v
                    Upstreams
                        |
                        v
                     Registry
                  +-----+-----+
                  |     |     |
                  v     v     v
               OpenAI Anthropic Google
                  |     |     |
                  +-----+-----+
                        |
                        v
                      Router
                        |
                 selects one
                        |
                        v
                    Upstream
                        |
                        v
                    Provider
                        |
                        v
                 External API
```

PostgreSQL and the Registry have different responsibilities:

```text
PostgreSQL
    = persistent configuration

Registry
    = in-memory collection of running Upstreams

Router
    = request-level selection
```

The application can load configuration from PostgreSQL and use it to create/register the appropriate `Upstream` objects. The Registry then provides those objects to the Router and other components such as the admin API and health checks.
---