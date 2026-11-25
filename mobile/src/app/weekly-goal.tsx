import { useEffect, useState } from "react";
import { Button, Chips, Notice, Screen, Txt } from "../components/ui";
import { useStore } from "../hooks/store";
import { featureStorage } from "../services/user-features";
import { colors } from "../theme";

export default function WeeklyGoal() {
  const s = useStore();
  const [goal, setGoal] = useState(3);
  const [ready, setReady] = useState(false);
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    if (!s.user) return;
    void featureStorage.weeklyGoal(s.user).then((value) => { setGoal(value); setReady(true); });
  }, [s.user]);
  async function save() {
    if (!s.user) return;
    await featureStorage.setWeeklyGoal(s.user, goal);
    setSaved(true);
  }
  return (
    <Screen>
      <Txt variant="title">Target mingguan</Txt>
      <Txt color={colors.muted}>Pilih jumlah sesi workout selesai yang ingin Anda capai tiap minggu. Target ini disimpan di perangkat dan tidak dikirim ke server.</Txt>
      <Chips values={[1, 2, 3, 4, 5, 6, 7].map(String)} selected={String(goal)} names={Object.fromEntries([1,2,3,4,5,6,7].map((value) => [String(value), `${value} sesi`]))} onSelect={(value) => { setGoal(Number(value)); setSaved(false); }} />
      {saved && <Notice message="Target mingguan tersimpan." />}
      <Button title="Simpan target" disabled={!ready} onPress={() => void save()} />
    </Screen>
  );
}
