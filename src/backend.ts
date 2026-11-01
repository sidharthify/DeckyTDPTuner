import { callable, toaster } from "@decky/api";
import { ChargerSettings, Profile, Resp, StatusData } from "./types";

export const getStatus = callable<[appid: string], Resp<StatusData>>("status");
export const tune = callable<[appid: string, target: number], Resp<null>>("tune");
export const stop = callable<[], Resp<null>>("stop");
export const forget = callable<[appid: string], Resp<null>>("forget");
export const apply = callable<[appid: string], Resp<Profile | null>>("apply");
export const reset = callable<[], Resp<null>>("reset");
export const setSettings = callable<[settings: ChargerSettings], Resp<null>>("set_settings");
export const live = callable<[appid: string, target: number, on: boolean], Resp<Profile>>("live");

// toast the error if the backend said no
export function check<T>(r: Resp<T>) {
  if (!r.ok) {
    toaster.toast({ title: "TDP Tuner", body: r.error ?? "failed" });
  }
  return r;
}
