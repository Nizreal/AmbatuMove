import { useLocalSearchParams } from "expo-router";
import { Screen, Notice } from "../../components/ui";
import { SessionSummary } from "../../components/session-summary";
import { useStore } from "../../hooks/store";
export default function Detail() {
  const { id } = useLocalSearchParams<{ id: string }>(),
    s = useStore().history.find((s) => s.id === id);
  return (
    <Screen>
      {s ? (
        <SessionSummary session={s} />
      ) : (
        <Notice message="Sesi tidak ditemukan di perangkat ini." />
      )}
    </Screen>
  );
}
