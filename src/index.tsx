import {
  ButtonItem,
  Field,
  PanelSection,
  PanelSectionRow,
  Router,
  SliderField,
  staticClasses,
} from "@decky/ui";
import { callable, definePlugin, toaster } from "@decky/api";
import { useEffect, useState } from "react";
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
  result: number;
  message: string;
};

type Profile = { watts: number; target: number; tuned: number };

type StatusData = { status: Status; min: number; max: number; profile?: Profile };

const getStatus = callable<[appid: string], Resp<StatusData>>("status");
const tune = callable<[appid: string, target: number], Resp<null>>("tune");
const stop = callable<[], Resp<null>>("stop");
const forget = callable<[appid: string], Resp<null>>("forget");

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

  const onTune = async () => {
    const r = tuning ? await stop() : await tune(appid, target);
    if (!r.ok) {
      toaster.toast({ title: "TDP Tuner", body: r.error ?? "failed" });
    }
    refresh();
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
          <ButtonItem layout="below" disabled={!app} onClick={onTune}>
            {tuning ? "Stop" : "Find lowest TDP"}
          </ButtonItem>
        </PanelSectionRow>
        {st && st.state != "idle" && st.appid == appid && (
          <PanelSectionRow>
            <Field
              label={st.message}
              description={
                tuning
                  ? `${st.lo}-${st.hi}W left, last ${st.fps.toFixed(1)} fps (low ${st.low.toFixed(1)})`
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
    </>
  );
}

export default definePlugin(() => {
  return {
    name: "TDP Tuner",
    titleView: <div className={staticClasses.Title}>TDP Tuner</div>,
    content: <Content />,
    icon: <FaBolt />,
  };
});
