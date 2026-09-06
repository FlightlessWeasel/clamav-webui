import ConfEditor from "../components/ConfEditor";
import ImageScanSettings from "../components/ImageScanSettings";
import NotificationSettings from "../components/NotificationSettings";
import PasswordSettings from "../components/PasswordSettings";

export default function Settings() {
  return (
    <>
      <h1 className="text-lg font-semibold">Settings</h1>

      <ImageScanSettings />
      <NotificationSettings />
      <PasswordSettings />

      <h2 className="pt-2 text-sm font-semibold text-zinc-500">Daemon configuration (clamd.conf)</h2>
      <ConfEditor which="clamd" />

      <h2 className="pt-2 text-sm font-semibold text-zinc-500">Updater configuration (freshclam.conf)</h2>
      <ConfEditor which="freshclam" />
    </>
  );
}
