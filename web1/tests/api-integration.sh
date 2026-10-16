#!/usr/bin/env bash
# AgentPlane control-plane integration test.
#
# Exercises every endpoint the web client calls against a running gateway,
# using the exact request bodies the client sends, then cleans up after itself.
#
#   BASE_URL=http://127.0.0.1:3001 ADMIN_TOKEN=dev-admin-token-change-me \
#     bash tests/api-integration.sh
#
# Requires: curl, python3. No npm, no node.
set -uo pipefail

B="${BASE_URL:-http://127.0.0.1:3001}"
TOKEN="${ADMIN_TOKEN:-${AGENTPLANE_ADMIN_TOKEN:-dev-admin-token-change-me}}"
R="$(date +%s)$RANDOM"
pass=0; fail=0; LAST_BODY=""

c() { printf '\033[%sm%s\033[0m' "$1" "$2"; }

req() { # method path expected_status [body]
  local m="$1" p="$2" want="$3" body="${4-}" out code body_out mark
  if [ -n "$body" ]; then
    out=$(curl -s -m 10 -w $'\n%{http_code}' -X "$m" -H "Authorization: Bearer $TOKEN" \
          -H 'Content-Type: application/json' -d "$body" "$B$p")
  else
    out=$(curl -s -m 10 -w $'\n%{http_code}' -X "$m" -H "Authorization: Bearer $TOKEN" "$B$p")
  fi
  code="${out##*$'\n'}"; body_out="${out%$'\n'*}"; LAST_BODY="$body_out"
  if [ "$code" = "$want" ]; then pass=$((pass+1)); mark=$(c 32 "ok  ")
  else fail=$((fail+1)); mark=$(c 31 "FAIL"); fi
  printf '%s %-4s %-52s want=%s got=%s  %s\n' "$mark" "$m" "$p" "$want" "$code" \
    "$(printf '%s' "$body_out" | tr -d '\n' | head -c 90)"
}

# Read a (possibly nested, dot-separated) key from the last response.
jget() { printf '%s' "$LAST_BODY" | python3 -c "
import sys, json
try: d = json.load(sys.stdin)
except Exception: print(''); raise SystemExit
v = d
for k in '$1'.split('.'):
    v = v.get(k, '') if isinstance(v, dict) else ''
print(v if v is not None else '')"; }

echo "Gateway: $B"
echo

# §6 — a provider reload resolves its secret by name, so the secret must exist first.
echo "── provider secrets (§6) ──────────────────────────────────────────────"
req POST /admin/secrets/provider 200 '{"provider":"openai","api_key":"sk-integration-test"}'

echo
echo "── providers (§9.1) ───────────────────────────────────────────────────"
req POST /admin/providers 201 "{\"name\":\"it-prov-$R\",\"base_url\":\"https://api.openai.com/v1\",\"secret_key\":\"OPENAI_API_KEY\",\"provider_group\":\"primary\",\"weight\":1,\"supported_models\":[\"gpt-4o\"],\"timeout_ms\":60000,\"max_retries\":1,\"breaker_failure_threshold\":5,\"enabled\":true}"
PID=$(jget ID)
req GET  /admin/providers 200
req PUT  "/admin/providers/$PID" 200 "{\"name\":\"it-prov-$R\",\"base_url\":\"https://api.openai.com/v1\",\"secret_key\":\"OPENAI_API_KEY\",\"provider_group\":\"primary\",\"weight\":5,\"supported_models\":[\"gpt-4o\",\"gpt-4o-mini\"],\"timeout_ms\":30000,\"max_retries\":0,\"breaker_failure_threshold\":3,\"enabled\":true}"
req POST "/admin/providers/$PID/disable" 200
req POST "/admin/providers/$PID/enable" 200
req GET  /admin/upstreams 200

echo
echo "── routing rules (§9.2) ───────────────────────────────────────────────"
req POST /admin/routing-rules 201 "{\"name\":\"it-rule-$R\",\"priority\":10,\"model_matcher\":{\"model\":\"gpt-4o\"},\"role_matcher\":{},\"user_matcher\":{},\"action\":\"route\",\"provider_group\":\"primary\",\"timeout_retry_overrides\":{},\"enabled\":true}"
RID=$(jget ID)
req GET  /admin/routing-rules 200
req PUT  "/admin/routing-rules/$RID" 200 "{\"name\":\"it-rule-$R\",\"priority\":3,\"model_matcher\":{},\"role_matcher\":{},\"user_matcher\":{},\"action\":\"route\",\"provider_group\":\"primary\",\"timeout_retry_overrides\":{},\"enabled\":true}"
req POST "/admin/routing-rules/$RID/disable" 200
req POST "/admin/routing-rules/$RID/enable" 200

echo
echo "── provisioning (§5) ──────────────────────────────────────────────────"
req POST /provision/user 201 "{\"username\":\"it-$R\",\"email\":\"it-$R@example.com\",\"key_name\":\"integration\"}"
UID_=$(jget user_id); KID=$(jget key_id); APIKEY=$(jget api_key)
echo "    user_id=$UID_ key_id=$KID"
req GET  /admin/api-keys 200

echo
echo "── IAM: policies (§8.1) ───────────────────────────────────────────────"
req POST /admin/policies 201 "{\"name\":\"it-policy-$R\",\"description\":\"integration\",\"document\":{\"Version\":\"2026-01-01\",\"Statement\":[{\"Sid\":\"AllowInvoke\",\"Effect\":\"Allow\",\"Action\":[\"ap:InvokeChat\"],\"Resource\":[\"arn:ap:model/*\"]}]}}"
req GET  /admin/policies 200
req GET  "/admin/policies/it-policy-$R/attachments" 200
req POST "/admin/policies/it-policy-$R/attach" 200 "{\"principal_type\":\"user\",\"principal_id\":\"$UID_\"}"
req POST "/admin/policies/it-policy-$R/detach" 200 "{\"principal_type\":\"user\",\"principal_id\":\"$UID_\"}"

echo
echo "── IAM: groups (§8.3) ─────────────────────────────────────────────────"
req POST /admin/groups 201 "{\"name\":\"it-group-$R\",\"description\":\"integration\"}"
req POST "/admin/groups/it-group-$R/members" 200 "{\"user_id\":\"$UID_\"}"
req GET  "/admin/groups/it-group-$R/members" 200
req DELETE "/admin/groups/it-group-$R/members/$UID_" 200
req GET  /admin/groups 200

echo
echo "── IAM: roles (§8.2) ──────────────────────────────────────────────────"
req POST /admin/roles 201 "{\"name\":\"it-role-$R\",\"description\":\"integration\"}"
req GET  /admin/roles 200
req POST "/admin/roles/it-role-$R/grant" 200 "{\"principal_type\":\"user\",\"principal_id\":\"$UID_\"}"
req GET  "/admin/roles/it-role-$R/grants" 200
req POST "/admin/roles/it-role-$R/revoke" 200 "{\"principal_type\":\"user\",\"principal_id\":\"$UID_\"}"

echo
echo "── IAM: users (§8.4) ──────────────────────────────────────────────────"
req GET  /admin/users 200
req POST "/admin/users/$UID_/roles" 200 '{"role":"admin"}'
req DELETE "/admin/users/$UID_/roles/admin" 200
req POST "/admin/users/$UID_/permissions-boundary" 200 '{"policy":""}'

echo
echo "── chat (§3) with the minted key ──────────────────────────────────────"
out=$(curl -s -m 15 -w $'\n%{http_code}' -X POST -H "Authorization: Bearer $APIKEY" \
      -H 'Content-Type: application/json' -d '{"model":"gpt-4o","messages":[{"role":"user","content":"ping"}]}' "$B/chat")
code="${out##*$'\n'}"
# 200 = answered, 403 = IAM denied (deny-by-default), 502 = upstream unreachable.
if [ "$code" = "200" ] || [ "$code" = "403" ] || [ "$code" = "502" ]; then
  pass=$((pass+1)); printf '%s %-4s %-52s got=%s\n' "$(c 32 'ok  ')" POST /chat "$code"
else
  fail=$((fail+1)); printf '%s %-4s %-52s got=%s  %s\n' "$(c 31 'FAIL')" POST /chat "$code" "${out%$'\n'*}"
fi

echo
echo "── observability (§10.1) ──────────────────────────────────────────────"
req GET /api/observability/health 200
req GET "/api/observability/overview?range=24h" 200
req GET "/api/observability/breakdowns?range=24h" 200
req GET "/api/observability/guardrails?range=24h&limit=50" 200
req GET "/api/observability/users?range=24h&limit=100" 200
req GET "/api/observability/prompts?range=24h&limit=50" 200

echo
echo "── observability ranges (the UI selector) ─────────────────────────────"
for r in 15m 1h 6h 24h 7d; do req GET "/api/observability/breakdowns?range=$r" 200; done
req GET "/api/observability/breakdowns?range=30d" 400   # not a supported range

echo
echo "── routing analytics (§10.2) ──────────────────────────────────────────"
for t in provider_traffic latency_percentiles error_rate_summary failover_count failover_transitions tokens_by_route tokens_by_provider cost_by_route cost_by_provider; do
  req GET "/admin/analytics?type=$t&hours=24" 200
done

echo
echo "── metrics (§7) ───────────────────────────────────────────────────────"
req GET /metrics 200

echo
echo "── cleanup ────────────────────────────────────────────────────────────"
req POST /admin/api-keys/revoke 200 "{\"key_id\":\"$KID\"}"
# Deleting the user also removes any remaining keys and IAM bindings, so a run
# leaves no user rows behind.
req DELETE "/admin/users/$UID_" 204
req DELETE "/admin/policies/it-policy-$R" 200
req DELETE "/admin/groups/it-group-$R" 200
req DELETE "/admin/roles/it-role-$R" 200
req DELETE "/admin/providers/$PID" 204
req DELETE "/admin/routing-rules/$RID" 204

echo
if [ "$fail" -eq 0 ]; then c 32 "PASS=$pass FAIL=0"; echo; else c 31 "PASS=$pass FAIL=$fail"; echo; exit 1; fi
