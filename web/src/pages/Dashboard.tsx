import { Card } from "../components/ui";

export default function Dashboard() {
  return (
    <>
      <h1 className="text-lg font-semibold">Dashboard</h1>
      <Card title="Status">
        <p className="text-sm text-zinc-500">
          Dashboard widgets (ClamAV version, service states, signature age, last scan) land with the ClamAV
          integration step.
        </p>
      </Card>
    </>
  );
}
