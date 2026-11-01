import { Field, PanelSection, PanelSectionRow, Router } from "@decky/ui";
import { useEffect, useState } from "react";
import { getStatus } from "../backend";
import { StatusData } from "../types";
import { AdvancedSection } from "./AdvancedSection";
import { ChargerSection } from "./ChargerSection";
import { GameSection } from "./GameSection";
import { TuneSection } from "./TuneSection";

export function Content() {
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
