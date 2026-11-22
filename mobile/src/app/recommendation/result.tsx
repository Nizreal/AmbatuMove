import { router } from "expo-router";
import { Button, Notice, Screen, Txt } from "../../components/ui";
import { PlanRow } from "../../components/exercise";
import { useStore } from "../../hooks/store";
import { begin, estimate } from "../../domain/workout";
import { colors } from "../../theme";

export default function Result() {
  const s = useStore();
  const p = s.plan;
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
      <Notice message="Pratinjau dibuat oleh AmbatuMove tanpa menyimpan akun atau workout di server. Riwayat sesi akan disimpan di perangkat." />
      {p.warnings?.map((warning, index) => (
        <Notice key={index} message={warning} />
      ))}
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
