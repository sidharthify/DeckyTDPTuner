import asyncio
import json
import os

import decky

BIN = os.path.join(decky.DECKY_PLUGIN_DIR, "bin", "tdptuner")
SOCK = os.path.join(decky.DECKY_PLUGIN_RUNTIME_DIR, "tdptuner.sock")
PROFILES = os.path.join(decky.DECKY_PLUGIN_SETTINGS_DIR, "profiles.json")
LOG = os.path.join(decky.DECKY_PLUGIN_LOG_DIR, "tdptuner.log")


class Plugin:
    proc = None

    async def _spawn(self):
        if self.proc and self.proc.returncode is None:
            return
        decky.logger.info(f"starting {BIN}")
        log = open(LOG, "a")
        self.proc = await asyncio.create_subprocess_exec(
            BIN, "-sock", SOCK, "-profiles", PROFILES,
            stdout=log, stderr=log,
        )
        # give it a sec to make the socket
        for _ in range(20):
            if os.path.exists(SOCK):
                break
            await asyncio.sleep(0.1)

    async def _call(self, cmd, **kw):
        await self._spawn()
        req = dict(kw, cmd=cmd)
        try:
            r, w = await asyncio.open_unix_connection(SOCK)
            w.write((json.dumps(req) + "\n").encode())
            await w.drain()
            line = await r.readline()
            w.close()
            return json.loads(line)
        except Exception as e:
            decky.logger.error(f"{cmd} failed: {e}")
            return {"ok": False, "error": str(e)}

    async def status(self, appid: str = ""):
        return await self._call("status", appid=str(appid))

    async def tune(self, appid: str, target: int):
        return await self._call("tune", appid=str(appid), target=int(target))

    async def stop(self):
        return await self._call("stop")

    async def apply(self, appid: str):
        return await self._call("apply", appid=str(appid))

    async def reset(self):
        return await self._call("reset")

    async def forget(self, appid: str):
        return await self._call("forget", appid=str(appid))

    async def _main(self):
        await self._spawn()

    async def _unload(self):
        if self.proc and self.proc.returncode is None:
            self.proc.terminate()
            try:
                await asyncio.wait_for(self.proc.wait(), 5)
            except asyncio.TimeoutError:
                self.proc.kill()
        decky.logger.info("tdptuner stopped")

    async def _uninstall(self):
        pass
