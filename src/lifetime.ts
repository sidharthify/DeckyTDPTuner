import { Router } from "@decky/ui";
import { toaster } from "@decky/api";
import { apply, reset, stop } from "./backend";

async function applyFor(appid: string) {
  const r = await apply(appid);
  if (r.ok && r.data) {
    toaster.toast({ title: "TDP Tuner", body: `Set ${r.data.watts}W (tuned for ${r.data.target} fps)` });
  }
}

// apply the saved tdp when a game starts, put things back when it exits
export function watchGames() {
  // plugin got (re)loaded mid game, the backend doesnt know about it yet
  const app = Router.MainRunningApp;
  if (app) {
    applyFor(String(app.appid));
  }

  return SteamClient.GameSessions.RegisterForAppLifetimeNotifications(async (n) => {
    if (!n.bRunning) {
      await stop();
      await reset();
      return;
    }
    await applyFor(String(n.unAppID));
  });
}
