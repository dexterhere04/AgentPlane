import { Icon } from '../icons';
import { Badge } from './ui';
import VaultView from './VaultView';
import ObservabilityView from './ObservabilityView';
import { Collapsible } from './RequestActivity';

export default function Diagnostics() {
  return (
    <div className="stack">
      <Collapsible
        title="Secrets & API keys"
        subtitle="Store provider credentials in Vault, provision user keys, and review what has been issued."
        badge={<Badge tone="accent"><Icon name="lock" size={11} /> vault</Badge>}
        defaultOpen
      >
        <VaultView />
      </Collapsible>

      <Collapsible
        title="ClickHouse analytics"
        subtitle="Traces, tokens, cost, models, and guardrail actions over the last 24 hours."
        badge={<Badge tone="neutral">/metrics</Badge>}
      >
        <ObservabilityView />
      </Collapsible>
    </div>
  );
}