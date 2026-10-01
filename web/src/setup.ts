const SETUP_KEY = 'agentplane.setup';

export interface SetupState {
  providerKey: boolean;
  provisioned: boolean;
  firstRequest: boolean;
  dismissed: boolean;
}

const DEFAULTS: SetupState = {
  providerKey: false,
  provisioned: false,
  firstRequest: false,
  dismissed: false
};

export function getSetup(): SetupState {
  try {
    const raw = localStorage.getItem(SETUP_KEY);
    if (!raw) return { ...DEFAULTS };
    return { ...DEFAULTS, ...(JSON.parse(raw) as Partial<SetupState>) };
  } catch {
    return { ...DEFAULTS };
  }
}

export function markSetup(patch: Partial<SetupState>): void {
  try {
    localStorage.setItem(SETUP_KEY, JSON.stringify({ ...getSetup(), ...patch }));
  } catch {
    /* storage unavailable — non-fatal */
  }
}

export function resetSetup(): void {
  try {
    localStorage.removeItem(SETUP_KEY);
  } catch {
    /* non-fatal */
  }
}
