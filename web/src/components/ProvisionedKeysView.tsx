import { useCallback, useEffect, useState } from 'react';
import { APIKeyItem, fetchAPIKeys } from '../api';
import { Alert, Badge, Card, EmptyState } from './ui';
import { Icon } from '../icons';

const VISIBLE_CHARS = 3;

function maskKey(keyId: string): string {
  const head = keyId.slice(0, VISIBLE_CHARS);
  const hidden = Math.max(keyId.length - VISIBLE_CHARS, 8);
  return head + '•'.repeat(hidden);
}

export default function ProvisionedKeysView() {
  const [keys, setKeys] = useState<APIKeyItem[]>([]);
  const [err, setErr] = useState('');
  const [loading, setLoading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setErr('');
    const res = await fetchAPIKeys();
    if (res.ok) setKeys(res.keys);
    else setErr('HTTP ' + res.status + ' · could not load keys');
    setLoading(false);
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <Card
      title="Provisioned API keys"
      subtitle="GET /admin/api-keys — every key minted so far and the user it belongs to. Only the first three characters are shown; the rest is masked."
      right={
        <button className="btn ghost sm" onClick={load} disabled={loading}>
          <Icon name="refresh" size={14} />
          {loading ? 'Loading…' : 'Refresh'}
        </button>
      }
    >
      {err && <Alert tone="error">{err}</Alert>}

      {keys.length === 0 ? (
        <EmptyState icon={<Icon name="key" size={20} />} text="No API keys provisioned yet." />
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>Key</th>
              <th>User</th>
              <th>Name</th>
              <th>Status</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {keys.map((k) => (
              <tr key={k.key_id}>
                <td><span className="key-masked">{maskKey(k.key_id)}</span></td>
                <td><span className="key-user">{k.username}</span></td>
                <td><span className="key-name">{k.name}</span></td>
                <td>
                  <Badge tone={k.revoked_at ? 'neutral' : 'pass'}>
                    {k.revoked_at ? 'revoked' : k.status}
                  </Badge>
                </td>
                <td><span className="key-created">{k.created_at.slice(0, 10)}</span></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}