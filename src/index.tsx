import {
  ButtonItem,
  Field,
  PanelSection,
  PanelSectionRow,
  Router,
  SliderField,
  ToggleField,
  staticClasses,
} from "@decky/ui";
import { callable, definePlugin, toaster } from "@decky/api";
import { useEffect, useRef, useState } from "react";
import { FaBolt } from "react-icons/fa";

type Resp<T> = { ok: boolean; error?: string; data?: T };

type Status = {
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

type Profile = { watts: number; target: number; tuned: number; live: boolean };

type GovStatus = {
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

type ChargerSettings = {
  full_power: boolean;
  watts: number;
  gpu_clock: number;
  pin_gpu: boolean;
};

type Charger = {
  ac: boolean;
  boosted: boolean;
  settings: ChargerSettings;
  watts: number;
  gpu_min: number;
  gpu_max: number;
};

type StatusData = {
  status: Status;
  governor: GovStatus;
  charger: Charger;
  min: number;
  max: number;
  profile?: Profile;
};

const getStatus = callable<[appid: string], Resp<StatusData>>("status");
const tune = callable<[appid: string, target: number], Resp<null>>("tune");
const stop = callable<[], Resp<null>>("stop");
const forget = callable<[appid: string], Resp<null>>("forget");
const apply = callable<[appid: string], Resp<Profile | null>>("apply");
const reset = callable<[], Resp<null>>("reset");
const setSettings = callable<[settings: ChargerSettings], Resp<null>>("set_settings");
const live = callable<[appid: string, target: number, on: boolean], Resp<Profile>>("live");

function Content() {
  const app = Router.MainRunningApp;
  const appid = app ? String(app.appid) : "";

  const [data, setData] = useState<StatusData>();
  const [target, setTarget] = useState(60);
  const [err, setErr] = useState("");

  const refresh = async () => {
    const r = await getStatus(appid);
    if (!r.ok) {
      setErr(r.error ?? "backend not responding");
      return;
    }
    setErr("");
    setData(r.data);
  };

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 1000);
    return () => clearInterval(t);
  }, [appid]);

  // start the slider at whatever we tuned for last time
  useEffect(() => {
    if (data?.profile) {
      setTarget(data.profile.target);
    }
  }, [data?.profile?.target]);

  const st = data?.status;
  const tuning = st?.state == "tuning";
  const unreachable = st?.state == "failed" && st.unreachable && st.appid == appid;
  const warn = { color: "#ff6b6b", fontWeight: "bold" };

  // the row is easy to miss, so shout once when it happens
  const prevState = useRef(st?.state);
  useEffect(() => {
    if (prevState.current == "tuning" && unreachable && st) {
      toaster.toast({
        title: `${data?.max}W can't hit ${st.target} fps`,
        body: `Best it managed was ${st.fps.toFixed(0)} fps`,
      });
    }
    prevState.current = st?.state;
  }, [st?.state]);

  const onTune = async () => {
    const r = tuning ? await stop() : await tune(appid, target);
    if (!r.ok) {
      toaster.toast({ title: "TDP Tuner", body: r.error ?? "failed" });
    }
    refresh();
  };

  const gov = data?.governor;
  const govHere = gov?.running && gov.appid == appid;

  const onLive = async (on: boolean) => {
    const r = await live(appid, target, on);
    if (!r.ok) {
      toaster.toast({ title: "TDP Tuner", body: r.error ?? "failed" });
    }
    refresh();
  };

  // local copy so sliders dont jump around while the 1s poll catches up
  const [cs, setCs] = useState<ChargerSettings>();
  useEffect(() => {
    if (!cs && data?.charger) {
      setCs(data.charger.settings);
    }
  }, [data?.charger]);

  const ch = data?.charger;
  const updateCharger = async (patch: Partial<ChargerSettings>) => {
    if (!cs) {
      return;
    }
    const next = { ...cs, ...patch };
    setCs(next);
    const r = await setSettings(next);
    if (!r.ok) {
      toaster.toast({ title: "TDP Tuner", body: r.error ?? "failed" });
    }
  };

  const onForget = async () => {
    await forget(appid);
    refresh();
  };

  if (err) {
    return (
      <PanelSection>
        <PanelSectionRow>
          <Field label="Backend error" description={err} />
        </PanelSectionRow>
      </PanelSection>
    );
  }

  return (
    <>
      <PanelSection title={app ? app.display_name : "No game running"}>
        <PanelSectionRow>
          <Field label="Current TDP">{st ? `${st.watts}W` : "..."}</Field>
        </PanelSectionRow>
        {data?.profile && (
          <PanelSectionRow>
            <Field label="Saved" description={`tuned for ${data.profile.target} fps`}>
              {data.profile.watts}W
            </Field>
          </PanelSectionRow>
        )}
      </PanelSection>

      <PanelSection title="Tune">
        <PanelSectionRow>
          <SliderField
            label="Target FPS"
            description="Set Steam's framerate limit to the same value"
            value={target}
            min={20}
            max={90}
            step={5}
            showValue
            disabled={tuning}
            onChange={setTarget}
          />
        </PanelSectionRow>
        <PanelSectionRow>
          <ToggleField
            label="Adjust live"
            description="Keep raising and lowering TDP while you play, starting from the tuned value"
            checked={!!data?.profile?.live}
            disabled={!app}
            onChange={onLive}
          />
        </PanelSectionRow>
        {govHere && (
          <PanelSectionRow>
            <Field
              label={
                gov.maxed ? (
                  <span style={warn}>{gov.watts}W isn't enough for {gov.target} fps here</span>
                ) : (
                  gov.message || `${gov.watts}W, floor ${gov.floor}W`
                )
              }
              description={`${gov.fps.toFixed(1)} fps${gov.gpu >= 0 ? `, gpu ${gov.gpu.toFixed(0)}%` : ""}, ${gov.raises} up / ${gov.drops} down`}
            />
          </PanelSectionRow>
        )}
        <PanelSectionRow>
          <ButtonItem layout="below" disabled={!app} onClick={onTune}>
            {tuning ? "Stop" : "Find lowest TDP"}
          </ButtonItem>
        </PanelSectionRow>
        {unreachable && st && (
          <>
            <PanelSectionRow>
              <Field
                label={<span style={warn}>{data?.max}W can't hit {st.target} fps</span>}
                description={`Even at max it only managed ${st.fps.toFixed(1)} fps (low ${st.low.toFixed(1)}). Nothing was saved.`}
              />
            </PanelSectionRow>
            {st.suggest > 0 && (
              <PanelSectionRow>
                <ButtonItem layout="below" onClick={() => setTarget(st.suggest)}>
                  Use {st.suggest} fps instead
                </ButtonItem>
              </PanelSectionRow>
            )}
          </>
        )}
        {st && st.state != "idle" && !unreachable && st.appid == appid && (
          <PanelSectionRow>
            <Field
              label={st.message}
              description={
                tuning
                  ? `${st.lo}-${st.hi}W left, last ${st.fps.toFixed(1)} fps (low ${st.low.toFixed(1)}, jitter ${st.jitter.toFixed(2)}ms)`
                  : undefined
              }
            />
          </PanelSectionRow>
        )}
        {data?.profile && !tuning && (
          <PanelSectionRow>
            <ButtonItem layout="below" onClick={onForget}>
              Forget saved TDP
            </ButtonItem>
          </PanelSectionRow>
        )}
      </PanelSection>

      {ch && cs && (
        <PanelSection title="On charger">
          <PanelSectionRow>
            <ToggleField
              label="Full power on charger"
              description={ch.boosted ? `Plugged in, running at ${ch.watts}W` : ch.ac ? "Plugged in" : "On battery"}
              checked={cs.full_power}
              onChange={(v) => updateCharger({ full_power: v })}
            />
          </PanelSectionRow>
          {cs.full_power && (
            <>
              <PanelSectionRow>
                <SliderField
                  label="TDP on charger"
                  value={cs.watts > 0 ? cs.watts : data!.max}
                  min={data!.min}
                  max={data!.max}
                  step={1}
                  showValue
                  valueSuffix="W"
                  onChange={(v) => updateCharger({ watts: v })}
                />
              </PanelSectionRow>
              {ch.gpu_max > 0 && (
                <PanelSectionRow>
                  <ToggleField
                    label="Pin GPU clock"
                    checked={cs.pin_gpu}
                    onChange={(v) => updateCharger({ pin_gpu: v })}
                  />
                </PanelSectionRow>
              )}
              {ch.gpu_max > 0 && cs.pin_gpu && (
                <PanelSectionRow>
                  <SliderField
                    label="GPU clock"
                    value={cs.gpu_clock > 0 ? cs.gpu_clock : ch.gpu_max}
                    min={ch.gpu_min}
                    max={ch.gpu_max}
                    step={100}
                    showValue
                    valueSuffix="MHz"
                    onChange={(v) => updateCharger({ gpu_clock: v })}
                  />
                </PanelSectionRow>
              )}
            </>
          )}
        </PanelSection>
      )}
    </>
  );
}

export default definePlugin(() => {
  const lifetime = SteamClient.GameSessions.RegisterForAppLifetimeNotifications(async (n) => {
    const appid = String(n.unAppID);
    if (!n.bRunning) {
      await stop();
      await reset();
      return;
    }

    const r = await apply(appid);
    if (r.ok && r.data) {
      toaster.toast({ title: "TDP Tuner", body: `Set ${r.data.watts}W (tuned for ${r.data.target} fps)` });
    }
  });

  return {
    name: "TDP Tuner",
    titleView: <div className={staticClasses.Title}>TDP Tuner</div>,
    content: <Content />,
    icon: <FaBolt />,
    onDismount() {
      lifetime.unregister();
    },
  };
});
