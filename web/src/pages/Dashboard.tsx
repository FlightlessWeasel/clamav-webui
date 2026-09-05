import { Link } from "react-router-dom";
import { getDashboard } from "../api/client";
import { useAsync } from "../lib/useAsync";
import { useEvents } from "../lib/useEvents";
import { humanCount, humanDuration } from "../lib/format";
import { Alert, Card, Spinner } from "../components/ui";
import ServiceCard from "../components/ServiceCard";

export default function Dashboard() {
  const { data, error, loading, reload } = useAsync(() => getDashboard(), []);
  useEvents(() => reload(), ["job", "service"]);

  if (loading && !data) return <Spinner />;
  if (error) return <Alert>{error.message}</Alert>;
  if (!data) return null;

  const { install, services } = data;

  return (
    <>
      <h1 className="text-lg font-semibold">Dashboard</h1>

      <Card
        title="ClamAV"
        actions={
          <Link className="text-xs text-sky-600 hover:underline" to="/install">
            {install.installed ? "Manage" : "Install"}
          </Link>
        }
      >
        {install.installed ? (
          <div className="space-y-1 text-sm">
            <div>
              Engine <span className="font-mono">{install.engine_version || "?"}</span>
              {install.db_version && (
                <>
                  {" "}
                  · signatures <span className="font-mono">{install.db_version}</span>
                </>
              )}
            </div>
            {install.db_date && <div className="text-xs text-zinc-500">DB built {install.db_date}</div>}
            {install.upgrade_available && (
              <div className="pt-1">
                <Alert kind="info">
                  Upgrade available: {install.apt_installed} → {install.apt_candidate}.{" "}
                  <Link className="underline" to="/install">
                    Upgrade
                  </Link>
                </Alert>
              </div>
            )}
          </div>
        ) : (
          <Alert kind="info">
            ClamAV is not installed.{" "}
            <Link className="underline" to="/install">
              Install it now
            </Link>
            .
          </Alert>
        )}
      </Card>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {services.map((svc) => (
          <ServiceCard key={svc.unit} svc={svc} onChange={reload} />
        ))}
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <Card
          title="Signatures"
          actions={
            <Link className="text-xs text-sky-600 hover:underline" to="/signatures">
              Details
            </Link>
          }
        >
          {data.signatures ? (
            <>
              <p className="text-2xl font-semibold">
                {data.signatures.age_seconds < 0 ? "—" : humanDuration(data.signatures.age_seconds)}
              </p>
              <p className="text-xs text-zinc-500">
                since last update · {humanCount(data.signatures.total_sigs)} sigs
              </p>
            </>
          ) : (
            <p className="text-sm text-zinc-500">unavailable</p>
          )}
        </Card>
        <Card
          title="Last scan"
          actions={
            <Link className="text-xs text-sky-600 hover:underline" to="/scans">
              History
            </Link>
          }
        >
          {data.last_scan ? (
            <>
              <p className="text-sm">
                <span className={data.last_scan.infected ? "font-semibold text-red-600" : ""}>
                  {data.last_scan.infected} infected
                </span>{" "}
                / {data.last_scan.scanned} scanned
              </p>
              <p className="text-xs text-zinc-500">
                {data.last_scan.status}
                {data.last_scan.finished_at
                  ? ` · ${new Date(data.last_scan.finished_at + "Z").toLocaleString()}`
                  : ""}
              </p>
            </>
          ) : (
            <p className="text-sm text-zinc-500">No scans yet.</p>
          )}
        </Card>
        <Card title="Quarantine">
          <p className="text-2xl font-semibold">{data.quarantine_held}</p>
          <p className="text-xs text-zinc-500">files held</p>
        </Card>
      </div>
    </>
  );
}
