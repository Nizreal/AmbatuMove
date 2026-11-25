import { router } from "expo-router";
import { useState } from "react";
import { Button, Notice, Screen, Txt } from "../../components/ui";
import { PlanRow } from "../../components/exercise";
import { useStore } from "../../hooks/store";
import { describeApiError, generateWorkout } from "../../services/ambatumove";
import { begin, estimate } from "../../domain/workout";
import { colors } from "../../theme";

export default function Result() {
  const s = useStore();
  const [regenerating, setRegenerating] = useState(false);
  const [regenerationError, setRegenerationError] = useState("");
  const p = s.plan;
  async function regenerate() {
    if (!p?.constraints || regenerating || s.active) return;
    setRegenerating(true);
    setRegenerationError("");
    try {
      const next = await s.withAuth((token) =>
        generateWorkout(p.constraints!, s.catalog, token, { save: false }),
      );
      s.setPlan(next);
    } catch (error) {
      setRegenerationError(describeApiError(error));
    } finally {
      setRegenerating(false);
    }
  }
  if (!p)
    return (
      <Screen>
        <Notice message="Belum ada rencana latihan." />
        <Button
          title="Isi kondisi"
          onPress={() => router.replace("/recommendation/form")}
        />
      </Screen>
    );
  return (
    <Screen
      footer={
        <Button
          title={s.active ? "Lanjutkan workout" : "Mulai workout"}
          onPress={() => {
            if (!s.active) s.setActive(begin(p));
            router.push("/workout/active");
          }}
        />
      }
    >
      <Txt variant="title">{p.title}</Txt>
      <Txt color={colors.muted}>
        {p.items.length} gerakan · sekitar{" "}
        {p.backendEstimatedDurationSeconds
          ? Math.ceil(p.backendEstimatedDurationSeconds / 60)
          : estimate(p.items)}{" "}
        menit
      </Txt>
      <Notice message={p.backendWorkoutId ? "Rencana ini tersimpan pada akun AmbatuMove. Riwayat latihan baru dicatat setelah sesi diselesaikan dan disimpan." : "Rencana tersimpan sebagai draf pada perangkat. Sesi akan masuk ke riwayat akun setelah selesai disimpan."} />
      {p.warnings?.map((warning, index) => (
        <Notice key={index} message={warning} />
      ))}
      {regenerationError ? <Notice error message={`Rekomendasi sebelumnya tetap tersedia. ${regenerationError}`} /> : null}
      <Button secondary title="Buat ulang rekomendasi" busy={regenerating} disabled={!!s.active || !p.constraints} onPress={() => void regenerate()} />
      {p.items.map((item, index) => (
        <PlanRow key={`${item.exercise.id}-${index}`} item={item} index={index}>
          <Button
            secondary
            title="Ganti gerakan"
            onPress={() =>
              router.push({
                pathname: "/catalog",
                params: { swap: String(index) },
              })
            }
          />
        </PlanRow>
      ))}
      <Button
        secondary
        title="Ubah preferensi"
        onPress={() => router.navigate("/recommendation/form")}
      />
    </Screen>
  );
}
