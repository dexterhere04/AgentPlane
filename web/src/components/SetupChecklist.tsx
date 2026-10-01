import { useMemo, useState } from 'react';
import { StreamState } from '../types';
import { getSetup, markSetup, resetSetup } from '../setup';
import { Icon, IconName } from '../icons';

interface Step {
  id: string;
  title: string;
  body: string;
  icon: IconName;
  done: boolean;
  cta: string;
  action: 'settings' | string;
}

export default function SetupChecklist({
  adminToken,
  stream,
  onNavigate,
  onOpenSettings
}: {
  adminToken: string;
  stream: StreamState;
  onNavigate: (tab: string) => void;
  onOpenSettings: () => void;
}) {
  const [dismissed, setDismissed] = useState(() => getSetup().dismissed);

  const steps: Step[] = useMemo(() => {
    const s = getSetup();
    return [
      {
        id: 'admin',
        title: 'Set your admin token',
        body: 'Authorises provisioning, Vault writes and analytics. Stored locally in your browser.',
        icon: 'key',
        done: !!adminToken && adminToken !== 'dev-admin-token-change-me',
        cta: 'Open settings',
        action: 'settings'
      },
      {
        id: 'provider',
        title: 'Store your provider key',
        body: 'Save the upstream OpenAI key so the gateway can forward requests.',
        icon: 'lock',
        done: s.providerKey,
        cta: 'Keys & Vault',
        action: 'keys'
      },
      {
        id: 'user',
        title: 'Provision a user & API key',
        body: 'Create a user, mint an API key and assign a role — access is deny-by-default.',
        icon: 'users',
        done: s.provisioned,
        cta: 'Provision a user',
        action: 'keys'
      },
      {
        id: 'request',
        title: 'Send your first request',
        body: 'Use the Playground to push a prompt through auth, guardrails and the provider.',
        icon: 'play',
        done: s.firstRequest || stream.requests.length > 0,
        cta: 'Open Playground',
        action: 'chat'
      }
    ];
  }, [adminToken, stream.requests.length]);

  const doneCount = steps.filter((s) => s.done).length;
  const allDone = doneCount === steps.length;

  if (dismissed) return null;

  const go = (action: string) => {
    if (action === 'settings') onOpenSettings();
    else onNavigate(action);
  };

  const dismiss = () => {
    markSetup({ dismissed: true });
    setDismissed(true);
  };

  if (allDone) {
    return (
      <section className="setup-card card is-complete reveal">
        <div className="setup-complete">
          <span className="setup-complete-icon"><Icon name="check" size={18} /></span>
          <div className="setup-complete-body">
            <span className="setup-complete-title">Setup complete</span>
            <span className="setup-complete-desc">
              Admin token, provider key, a user with a role, and a first request — the gateway is fully wired up.
            </span>
          </div>
          <button
            className="btn ghost sm"
            onClick={() => {
              resetSetup();
              setDismissed(true);
            }}
          >
            <Icon name="refresh" size={14} />
            Reset guide
          </button>
          <button className="setup-dismiss" onClick={dismiss} aria-label="Dismiss">
            <Icon name="x" size={15} />
          </button>
        </div>
      </section>
    );
  }

  return (
    <section className="setup-card card reveal">
      <header className="setup-head">
        <div>
          <span className="setup-eyebrow">Getting started</span>
          <h3>Finish setting up AgentPlane</h3>
          <p>
            {doneCount} of {steps.length} steps complete — follow them in order to get a request flowing end to end.
          </p>
        </div>
        <button className="setup-dismiss" onClick={dismiss} aria-label="Dismiss setup guide">
          <Icon name="x" size={15} />
        </button>
      </header>

      <div className="setup-progress" role="progressbar" aria-valuenow={doneCount} aria-valuemin={0} aria-valuemax={steps.length}>
        <div className="setup-progress-fill" style={{ width: (doneCount / steps.length) * 100 + '%' }} />
      </div>

      <ol className="setup-steps">
        {steps.map((s, i) => (
          <li key={s.id} className={'setup-step' + (s.done ? ' done' : '')}>
            <span className="setup-step-num">{s.done ? <Icon name="check" size={13} /> : i + 1}</span>
            <span className="setup-step-icon"><Icon name={s.icon} size={16} /></span>
            <div className="setup-step-body">
              <span className="setup-step-title">{s.title}</span>
              <span className="setup-step-desc">{s.body}</span>
            </div>
            {s.done ? (
              <span className="badge pass">Done</span>
            ) : (
              <button className="btn sm" onClick={() => go(s.action)}>
                {s.cta}
                <Icon name="arrow" size={14} />
              </button>
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}
