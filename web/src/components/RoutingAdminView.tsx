import { useEffect, useState } from 'react';
import { Icon } from '../icons';
import {
  createProvider,
  createRoutingRule,
  deleteProvider,
  deleteRoutingRule,
  disableProvider,
  disableRoutingRule,
  enableProvider,
  enableRoutingRule,
  fetchProviders,
  fetchRoutingRules,
  ProviderRecord,
  RoutingRuleRecord,
  updateProvider,
  updateRoutingRule
} from '../api';
import {
  Alert,
  Badge,
  Card,
  EmptyState,
  LoadingState
} from './ui';

const emptyProvider: Omit<ProviderRecord, 'id'> = {
  name: '',
  base_url: '',
  secret_ref: '',
  provider_group: '',
  weight: 1,
  supported_models: [],
  timeout_ms: 60000,
  max_retries: 1,
  enabled: true
};

interface RoutingRuleForm {
  name: string;
  priority: number;
  model_matcher: string;
  role_matcher: string;
  user_matcher: string;
  action: string;
  provider_group: string;
  timeout_retry_overrides: string;
  enabled: boolean;
}

const emptyRule: RoutingRuleForm = {
  name: '',
  priority: 0,
  model_matcher: '{}',
  role_matcher: '{}',
  user_matcher: '{}',
  action: 'route',
  provider_group: '',
  timeout_retry_overrides: '{}',
  enabled: true
};

function prettyJSON(value: unknown): string {
  if (value == null) return '{}';

  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return '{}';
  }
}

function parseJSON(value: string, field: string): unknown {
  try {
    return JSON.parse(value || '{}');
  } catch {
    throw new Error(`${field} must contain valid JSON.`);
  }
}

export default function RoutingAdminView() {
  const [providers, setProviders] = useState<ProviderRecord[]>([]);
  const [rules, setRules] = useState<RoutingRuleRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
const [ruleForm, setRuleForm] = useState<RoutingRuleForm>(emptyRule);
  const [providerForm, setProviderForm] =
    useState<Omit<ProviderRecord, 'id'>>(emptyProvider);
  const [editingProvider, setEditingProvider] = useState<string | null>(null);

  const [editingRule, setEditingRule] = useState<string | null>(null);

  async function load() {
    setLoading(true);
    setError('');

    const [providerResult, ruleResult] = await Promise.all([
      fetchProviders(),
      fetchRoutingRules()
    ]);

    if (!providerResult.ok || !ruleResult.ok) {
      setError('Unable to load provider or routing-rule configuration.');
    }

    setProviders(providerResult.providers);
    setRules(ruleResult.rules);
    setLoading(false);
  }

  useEffect(() => {
    void load();
  }, []);

  async function saveProvider() {
    setSaving(true);
    setError('');

    try {
      const result = editingProvider
        ? await updateProvider(editingProvider, providerForm)
        : await createProvider(providerForm);

      if (!result.ok) {
        throw new Error(
          typeof result.body === 'string'
            ? result.body
            : `Provider request failed with HTTP ${result.status}.`
        );
      }

      setProviderForm(emptyProvider);
      setEditingProvider(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Provider save failed.');
    } finally {
      setSaving(false);
    }
  }

  async function saveRule() {
    setSaving(true);
    setError('');

    try {
      const rule: Omit<RoutingRuleRecord, 'id'> = {
  ...ruleForm,
  model_matcher: parseJSON(ruleForm.model_matcher, 'Model matcher'),
  role_matcher: parseJSON(ruleForm.role_matcher, 'Role matcher'),
  user_matcher: parseJSON(ruleForm.user_matcher, 'User matcher'),
  timeout_retry_overrides: parseJSON(
    ruleForm.timeout_retry_overrides,
    'Timeout/retry overrides'
  )
};

      const result = editingRule
        ? await updateRoutingRule(editingRule, rule)
        : await createRoutingRule(rule);

      if (!result.ok) {
        throw new Error(
          typeof result.body === 'string'
            ? result.body
            : `Routing-rule request failed with HTTP ${result.status}.`
        );
      }

      setRuleForm(emptyRule);
      setEditingRule(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Routing-rule save failed.');
    } finally {
      setSaving(false);
    }
  }

  async function toggleProvider(provider: ProviderRecord) {
    const result = provider.enabled
      ? await disableProvider(provider.id)
      : await enableProvider(provider.id);

    if (!result.ok) {
      setError(`Unable to change provider "${provider.name}".`);
      return;
    }

    await load();
  }

  async function toggleRule(rule: RoutingRuleRecord) {
    const result = rule.enabled
      ? await disableRoutingRule(rule.id)
      : await enableRoutingRule(rule.id);

    if (!result.ok) {
      setError(`Unable to change routing rule "${rule.name}".`);
      return;
    }

    await load();
  }

  async function removeProvider(provider: ProviderRecord) {
    if (!window.confirm(`Delete provider "${provider.name}"?`)) return;

    const result = await deleteProvider(provider.id);

    if (!result.ok) {
      setError(`Unable to delete provider "${provider.name}".`);
      return;
    }

    if (editingProvider === provider.id) {
      setEditingProvider(null);
      setProviderForm(emptyProvider);
    }

    await load();
  }

  async function removeRule(rule: RoutingRuleRecord) {
    if (!window.confirm(`Delete routing rule "${rule.name}"?`)) return;

    const result = await deleteRoutingRule(rule.id);

    if (!result.ok) {
      setError(`Unable to delete routing rule "${rule.name}".`);
      return;
    }

    if (editingRule === rule.id) {
      setEditingRule(null);
      setRuleForm(emptyRule);
    }

    await load();
  }

  function editProvider(provider: ProviderRecord) {
    setEditingProvider(provider.id);
    setProviderForm({
      name: provider.name,
      base_url: provider.base_url,
      secret_ref: provider.secret_ref || '',
      provider_group: provider.provider_group || '',
      weight: provider.weight,
      supported_models: provider.supported_models || [],
      timeout_ms: provider.timeout_ms,
      max_retries: provider.max_retries,
      enabled: provider.enabled
    });
  }

  function editRule(rule: RoutingRuleRecord) {
    setEditingRule(rule.id);
    setRuleForm({
      name: rule.name,
      priority: rule.priority,
      model_matcher: prettyJSON(rule.model_matcher),
      role_matcher: prettyJSON(rule.role_matcher),
      user_matcher: prettyJSON(rule.user_matcher),
      action: rule.action,
      provider_group: rule.provider_group || '',
      timeout_retry_overrides: prettyJSON(rule.timeout_retry_overrides),
      enabled: rule.enabled
    });
  }

  return (
    <div style={{ marginTop: 24 }}>
      {error && <Alert tone="warn">{error}</Alert>}

      <Card
        title="Provider / upstream administration"
        subtitle="Create and manage the upstreams used by routing and failover."
        right={
          <button
            className="btn ghost sm"
            onClick={() => {
              setEditingProvider(null);
              setProviderForm(emptyProvider);
            }}
          >
            <Icon name="plus" size={14} />
            New provider
          </button>
        }
      >
        <div className="grid-2">
          <div>
            {loading ? (
              <LoadingState text="Loading providers…" />
            ) : providers.length === 0 ? (
              <EmptyState
                icon={<Icon name="route" size={20} />}
                text="No providers configured."
              />
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Group</th>
                    <th>Models</th>
                    <th>Weight</th>
                    <th>Status</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {providers.map((provider) => (
                    <tr key={provider.id}>
                      <td>
                        <strong>{provider.name}</strong>
                        <div className="card-sub">{provider.base_url}</div>
                      </td>
                      <td>{provider.provider_group || '—'}</td>
                      <td>{provider.supported_models?.join(', ') || '—'}</td>
                      <td>{provider.weight}</td>
                      <td>
                        <Badge tone={provider.enabled ? 'pass' : 'neutral'}>
                          {provider.enabled ? 'ENABLED' : 'DISABLED'}
                        </Badge>
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: 6 }}>
                          <button
                            className="btn ghost sm"
                            onClick={() => editProvider(provider)}
                          >
                            Edit
                          </button>
                          <button
                            className="btn ghost sm"
                            onClick={() => void toggleProvider(provider)}
                          >
                            {provider.enabled ? 'Disable' : 'Enable'}
                          </button>
                          <button
                            className="btn ghost sm"
                            onClick={() => void removeProvider(provider)}
                          >
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <div className="card" style={{ boxShadow: 'none' }}>
            <div className="card-head">
              <div>
                <h3>{editingProvider ? 'Edit provider' : 'Add provider'}</h3>
                <span className="card-sub">
                  API credentials are referenced through the secret store.
                </span>
              </div>
            </div>

            <div className="card-body">
              <div className="form-grid">
                <label>
                  Name
                  <input
                    className="input"
                    value={providerForm.name}
                    onChange={(e) =>
                      setProviderForm({ ...providerForm, name: e.target.value })
                    }
                    placeholder="openai-primary"
                  />
                </label>

                <label>
                  Provider group
                  <input
                    className="input"
                    value={providerForm.provider_group}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        provider_group: e.target.value
                      })
                    }
                    placeholder="openai"
                  />
                </label>

                <label style={{ gridColumn: '1 / -1' }}>
                  Base URL
                  <input
                    className="input mono"
                    value={providerForm.base_url}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        base_url: e.target.value
                      })
                    }
                    placeholder="https://api.openai.com"
                  />
                </label>

                <label>
                  Secret reference
                  <input
                    className="input mono"
                    value={providerForm.secret_ref || ''}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        secret_ref: e.target.value
                      })
                    }
                    placeholder="openai_production"
                  />
                </label>

                <label>
                  Supported models
                  <input
                    className="input"
                    value={providerForm.supported_models.join(', ')}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        supported_models: e.target.value
                          .split(',')
                          .map((v) => v.trim())
                          .filter(Boolean)
                      })
                    }
                    placeholder="gpt-4o, gpt-4o-mini"
                  />
                </label>

                <label>
                  Weight
                  <input
                    className="input"
                    type="number"
                    min="1"
                    value={providerForm.weight}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        weight: Number(e.target.value)
                      })
                    }
                  />
                </label>

                <label>
                  Timeout (ms)
                  <input
                    className="input"
                    type="number"
                    min="0"
                    value={providerForm.timeout_ms}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        timeout_ms: Number(e.target.value)
                      })
                    }
                  />
                </label>

                <label>
                  Max retries
                  <input
                    className="input"
                    type="number"
                    min="0"
                    value={providerForm.max_retries}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        max_retries: Number(e.target.value)
                      })
                    }
                  />
                </label>

                <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input
                    type="checkbox"
                    checked={providerForm.enabled}
                    onChange={(e) =>
                      setProviderForm({
                        ...providerForm,
                        enabled: e.target.checked
                      })
                    }
                  />
                  Enabled
                </label>
              </div>

              <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
                <button
                  className="btn"
                  disabled={saving}
                  onClick={() => void saveProvider()}
                >
                  {saving ? 'Saving…' : editingProvider ? 'Update provider' : 'Create provider'}
                </button>

                {editingProvider && (
                  <button
                    className="btn ghost"
                    onClick={() => {
                      setEditingProvider(null);
                      setProviderForm(emptyProvider);
                    }}
                  >
                    Cancel
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>
      </Card>

      <Card
        title="Routing rule administration"
        subtitle="Match requests and select a provider group."
        right={
          <button
            className="btn ghost sm"
            onClick={() => {
              setEditingRule(null);
              setRuleForm(emptyRule);
            }}
          >
            <Icon name="plus" size={14} />
            New rule
          </button>
        }
      >
        <div className="grid-2">
          <div>
            {loading ? (
              <LoadingState text="Loading routing rules…" />
            ) : rules.length === 0 ? (
              <EmptyState
                icon={<Icon name="route" size={20} />}
                text="No routing rules configured."
              />
            ) : (
              <table className="table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Priority</th>
                    <th>Action</th>
                    <th>Provider group</th>
                    <th>Status</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {rules.map((rule) => (
                    <tr key={rule.id}>
                      <td><strong>{rule.name}</strong></td>
                      <td>{rule.priority}</td>
                      <td>{rule.action}</td>
                      <td>{rule.provider_group || '—'}</td>
                      <td>
                        <Badge tone={rule.enabled ? 'pass' : 'neutral'}>
                          {rule.enabled ? 'ENABLED' : 'DISABLED'}
                        </Badge>
                      </td>
                      <td>
                        <div style={{ display: 'flex', gap: 6 }}>
                          <button
                            className="btn ghost sm"
                            onClick={() => editRule(rule)}
                          >
                            Edit
                          </button>
                          <button
                            className="btn ghost sm"
                            onClick={() => void toggleRule(rule)}
                          >
                            {rule.enabled ? 'Disable' : 'Enable'}
                          </button>
                          <button
                            className="btn ghost sm"
                            onClick={() => void removeRule(rule)}
                          >
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          <div className="card" style={{ boxShadow: 'none' }}>
            <div className="card-head">
              <div>
                <h3>{editingRule ? 'Edit routing rule' : 'Add routing rule'}</h3>
                <span className="card-sub">
                  Matchers are stored as JSON and passed directly to the routing API.
                </span>
              </div>
            </div>

            <div className="card-body">
              <div className="form-grid">
                <label>
                  Name
                  <input
                    className="input"
                    value={ruleForm.name}
                    onChange={(e) =>
                      setRuleForm({ ...ruleForm, name: e.target.value })
                    }
                    placeholder="engineering-gpt4"
                  />
                </label>

                <label>
                  Priority
                  <input
                    className="input"
                    type="number"
                    value={ruleForm.priority}
                    onChange={(e) =>
                      setRuleForm({
                        ...ruleForm,
                        priority: Number(e.target.value)
                      })
                    }
                  />
                </label>

                <label>
                  Action
                  <input
                    className="input"
                    value={ruleForm.action}
                    onChange={(e) =>
                      setRuleForm({ ...ruleForm, action: e.target.value })
                    }
                    placeholder="route"
                  />
                </label>

                <label>
                  Provider group
                  <input
                    className="input"
                    value={ruleForm.provider_group || ''}
                    onChange={(e) =>
                      setRuleForm({
                        ...ruleForm,
                        provider_group: e.target.value
                      })
                    }
                    placeholder="openai"
                  />
                </label>

                <label style={{ gridColumn: '1 / -1' }}>
                  Model matcher
                  <textarea
                    className="input mono"
                    rows={4}
                    value={String(ruleForm.model_matcher ?? '{}')}
                    onChange={(e) =>
                      setRuleForm({
                        ...ruleForm,
                        model_matcher: e.target.value
                      })
                    }
                    placeholder='{"pattern":"gpt-4o"}'
                  />
                </label>

                <label style={{ gridColumn: '1 / -1' }}>
                  Role matcher
                  <textarea
                    className="input mono"
                    rows={3}
                    value={String(ruleForm.role_matcher ?? '{}')}
                    onChange={(e) =>
                      setRuleForm({
                        ...ruleForm,
                        role_matcher: e.target.value
                      })
                    }
                    placeholder='{"roles":["engineering"]}'
                  />
                </label>

                <label style={{ gridColumn: '1 / -1' }}>
                  User matcher
                  <textarea
                    className="input mono"
                    rows={3}
                    value={String(ruleForm.user_matcher ?? '{}')}
                    onChange={(e) =>
                      setRuleForm({
                        ...ruleForm,
                        user_matcher: e.target.value
                      })
                    }
                    placeholder='{"user_ids":["..."]}'
                  />
                </label>

                <label style={{ gridColumn: '1 / -1' }}>
                  Timeout / retry overrides
                  <textarea
                    className="input mono"
                    rows={4}
                    value={String(ruleForm.timeout_retry_overrides ?? '{}')}
                    onChange={(e) =>
                      setRuleForm({
                        ...ruleForm,
                        timeout_retry_overrides: e.target.value
                      })
                    }
                    placeholder='{"timeout_ms":30000,"max_retries":1}'
                  />
                </label>

                <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input
                    type="checkbox"
                    checked={ruleForm.enabled}
                    onChange={(e) =>
                      setRuleForm({
                        ...ruleForm,
                        enabled: e.target.checked
                      })
                    }
                  />
                  Enabled
                </label>
              </div>

              <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
                <button
                  className="btn"
                  disabled={saving}
                  onClick={() => void saveRule()}
                >
                  {saving ? 'Saving…' : editingRule ? 'Update rule' : 'Create rule'}
                </button>

                {editingRule && (
                  <button
                    className="btn ghost"
                    onClick={() => {
                      setEditingRule(null);
                      setRuleForm(emptyRule);
                    }}
                  >
                    Cancel
                  </button>
                )}
              </div>
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}