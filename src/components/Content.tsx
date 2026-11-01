import { Field, PanelSection, PanelSectionRow, Router } from "@decky/ui";
import { useEffect, useState } from "react";
import { getStatus } from "../backend";
import { StatusData } from "../types";
import { AdvancedSection } from "./AdvancedSection";
import { ChargerSection } from "./ChargerSection";
import { GameSection } from "./GameSection";
import { TuneSection } from "./TuneSection";

// decky unmounts the panel whenever the QAM closes, so the slider has to live
// out here or it snaps back to 60 every time
const targets = new Map<string, number>();

export function Content() {
  const app = Router.MainRunningApp;
  const appid = app ? String(app.appid) : "";

  const [data, setData] = useState<StatusData>();
  const [target, setTargetState] = useState(() => targets.get(appid) ?? 60);
  const setTarget = (t: number) => {
    targets.set(appid, t);
    setTargetState(t);
  };
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

  // start the slider at whatever we tuned for last time, unless it got moved
  useEffect(() => {
    if (data?.profile && !targets.has(appid)) {
      setTargetState(data.profile.target);
    }
  }, [data?.profile?.target]);

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
      <GameSection name={app?.display_name} data={data} />
      <TuneSection appid={appid} data={data} target={target} setTarget={setTarget} refresh={refresh} />
      {data?.charger && <ChargerSection data={data} />}
      {data?.charger && <AdvancedSection data={data} />}
    </>
  );
}
