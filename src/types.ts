export type Resp<T> = { ok: boolean; error?: string; data?: T };

export type Status = {
  state: "idle" | "tuning" | "done" | "failed";
  appid: string;
  target: number;
  watts: number;
  lo: number;
  hi: number;
  fps: number;
  low: number;
  jitter: number;
  result: number;
  unreachable: boolean;
  suggest: number;
  message: string;
};

export type Profile = { watts: number; target: number; tuned: number; live: boolean };

export type GovStatus = {
  running: boolean;
  appid: string;
  target: number;
  watts: number;
  floor: number;
  fps: number;
  gpu: number;
  raises: number;
  drops: number;
  maxed: boolean;
  message: string;
};

export type Settings = {
  full_power: boolean;
  watts: number;
  gpu_clock: number;
  pin_gpu: boolean;
  min_watts: number;
};

export type Charger = {
  ac: boolean;
  boosted: boolean;
  settings: Settings;
  watts: number;
  gpu_min: number;
  gpu_max: number;
  gpu_cap: number;
  gpu_full_watts: number;
  floor: number;
  default_min: number;
};

export type StatusData = {
  status: Status;
  governor: GovStatus;
  charger: Charger;
  min: number;
  max: number;
  profile?: Profile;
};
