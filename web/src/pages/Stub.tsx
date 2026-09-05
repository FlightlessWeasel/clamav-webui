import { Card } from "../components/ui";

// Stub stands in for pages that arrive in later build steps, so the nav never
// dead-ends.
export default function Stub({ title }: { title: string }) {
  return (
    <>
      <h1 className="text-lg font-semibold">{title}</h1>
      <Card>
        <p className="text-sm text-zinc-500">This section is not built yet.</p>
      </Card>
    </>
  );
}
