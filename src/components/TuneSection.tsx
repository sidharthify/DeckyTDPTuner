import { ButtonItem, Field, PanelSection, PanelSectionRow, SliderField, ToggleField } from "@decky/ui";
import { toaster } from "@decky/api";
import { useEffect, useRef } from "react";
import { check, forget, live, stop, tune } from "../backend";
import { StatusData } from "../types";

const warn = { color: "#ff6b6b", fontWeight: "bold" };

type Props = {
  appid: string;
  data?: StatusData;
  target: number;
  setTarget: (t: number) => void;
  refresh: () => void;
};

export function TuneSection({ appid, data, target, setTarget, refresh }: Props) {
  const st = data?.status;
  const tuning = st?.state == "tuning";
  const unreachable = st?.state == "failed" && st.unreachable && st.appid == appid;
  const gov = data?.governor;
  const govHere = gov?.running && gov.appid == appid;

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
    check(tuning ? await stop() : await tune(appid, target));
    refresh();
  };

  const onLive = async (on: boolean) => {
    check(await live(appid, target, on));
    refresh();
  };

  const onForget = async () => {
    await forget(appid);
    refresh();
  };

  return (
    <PanelSection title="Tune">
      <PanelSectionRow>
        <SliderField
          label="Target FPS"
          description="Set Steam's framerate limit to the same value"
          value={tuning && st ? st.target : target}
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
          disabled={!appid}
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
        <ButtonItem layout="below" disabled={!appid} onClick={onTune}>
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
          {st.suggest > 0 && target != st.suggest && (
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
  );
}
