import { useState } from "react";
import { View } from "react-native";
import { Alert } from "../../components/confirm";
import { Redirect, router, usePreventRemove } from "expo-router";
import { Button, Notice, Screen, Txt, styles } from "../../components/ui";
import { ExercisePhoto } from "../../components/exercise";
import { useStore } from "../../hooks/store";
import {
  CALORIE_REFERENCE_WEIGHT_KG,
  counts,
  estimatedCalories,
  finish,
  mark,
} from "../../domain/workout";
import { colors } from "../../theme";
export default function Active() {
  const s = useStore(),
    [index, setIndex] = useState(0),
    [leaving, setLeaving] = useState(false);
  const session = s.active;
  const disablePrevention = usePreventRemove(
    !!session && !session.finishedAt && !leaving,
    ({ repeat }) => {
      Alert.alert(
        "Tinggalkan sesi?",
        "Sesi tetap tersimpan dan bisa dilanjutkan dari beranda. Untuk mencatat hasil parsial, pilih Akhiri sesi.",
        [
          { text: "Tetap latihan", style: "cancel" },
          {
            text: "Simpan dan keluar",
            onPress: () => {
              setLeaving(true);
              repeat();
            },
          },
        ],
      );
    },
  );
  if (session?.finishedAt) return <Redirect href="/workout/summary" />;
  if (!session)
    return (
      <Screen>
        <Notice message="Tidak ada sesi aktif." />
        <Button
          title="Kembali ke beranda"
          onPress={() => router.replace("/")}
        />
      </Screen>
    );
  const current = Math.min(index, session.plan.items.length - 1),
    item = session.plan.items[current],
    actual = session.actual[current],
    stat = counts(session),
    movementCalories = estimatedCalories(
      item,
      actual.filter((status) => status === "completed").length,
    ),
    next = actual.findIndex((v) => v !== "completed");
  function finishSession() {
    Alert.alert(
      "Akhiri sesi latihan?",
      `${stat.completed} dari ${stat.total} set selesai. Set lainnya tetap tercatat sebagai dilewati atau belum selesai.`,
      [
        { text: "Lanjut latihan", style: "cancel" },
        {
          text: "Lihat hasil",
          onPress: () => {
            setLeaving(true);
            disablePrevention();
            s.setActive(finish(session!));
            router.replace("/workout/summary");
          },
        },
      ],
    );
  }
  return (
    <Screen
      footer={
        <Button
          title={
            next < 0
              ? current < session.plan.items.length - 1
                ? "Gerakan berikutnya"
                : "Selesaikan latihan"
              : `Selesaikan set ${next + 1}`
          }
          onPress={() => {
            if (next >= 0)
              s.setActive(mark(session, current, next, "completed"));
            else if (current < session.plan.items.length - 1)
              setIndex(current + 1);
            else finishSession();
          }}
        />
      }
    >
      <View style={styles.activeHeader}>
        <View style={styles.activeHeaderTop}>
          <View style={{ flex: 1 }}>
            <Txt variant="label" color={colors.surface}>
              {session.plan.title}
            </Txt>
            <Txt variant="small" color={colors.surface}>
              {session.plan.source === "manual"
                ? "Workout manual"
                : "Rekomendasi"}
            </Txt>
          </View>
          <Txt variant="label" color={colors.surface}>
            {current + 1}/{session.plan.items.length}
          </Txt>
        </View>
        <View style={styles.activeTrack}>
          <View
            style={[
              styles.activeProgress,
              {
                width: `${Math.min(100, (stat.completed / stat.total) * 100)}%`,
              },
            ]}
          />
        </View>
        <Txt variant="small" color={colors.surface}>
          {stat.completed} dari {stat.total} set selesai
        </Txt>
      </View>
      <Txt variant="title">{item.exercise.name}</Txt>
      <ExercisePhoto exercise={item.exercise} large />
      <View style={styles.targetPanel}>
        <View style={{ flex: 1 }}>
          <Txt variant="small" color={colors.muted}>
            Target per set
          </Txt>
          <Txt variant="heading">
            {item.target} {item.exercise.timed ? "detik" : "repetisi"}
          </Txt>
        </View>
        <View style={styles.targetDivider} />
        <View style={{ flex: 1 }}>
          <Txt variant="small" color={colors.muted}>
            Jumlah set
          </Txt>
          <Txt variant="heading">{item.sets} set</Txt>
        </View>
      </View>
      <Txt variant="small" color={colors.muted}>
        Perkiraan lokal: ≈ {movementCalories} kcal (acuan{" "}
        {CALORIE_REFERENCE_WEIGHT_KG} kg; bukan estimasi server)
      </Txt>
      <Txt color={colors.muted}>
        Jeda {item.rest} detik antarset.{" "}
        {item.exercise.timed
          ? "Gunakan jam Anda, lalu konfirmasi setelah durasi selesai."
          : "Konfirmasi hanya set yang benar-benar Anda lakukan."}
      </Txt>
      {item.exercise.instructions.map((v, i) => (
        <Txt key={i}>
          {i + 1}. {v}
        </Txt>
      ))}
      <View style={styles.setList}>
        {actual.map((status, i) => (
          <View key={i} style={styles.row}>
            <Txt variant="label">Set {i + 1}</Txt>
            <Txt color={status === "completed" ? colors.success : colors.muted}>
              {status === "completed"
                ? "✓ Selesai"
                : status === "skipped"
                  ? "Dilewati"
                  : "Belum selesai"}
            </Txt>
            {status === "completed" && (
              <Button
                secondary
                title={`Batalkan set ${i + 1}`}
                onPress={() =>
                  s.setActive(mark(session, current, i, "pending"))
                }
              />
            )}
          </View>
        ))}
      </View>
      <Button
        secondary
        title="Lewati set tersisa gerakan ini"
        disabled={actual.every((x) => x !== "pending")}
        onPress={() => {
          s.setActive({
            ...session,
            actual: session.actual.map((row, i) =>
              i === current
                ? row.map((v) => (v === "pending" ? "skipped" : v))
                : row,
            ),
          });
          if (current < session.plan.items.length - 1) setIndex(current + 1);
        }}
      />
      <View style={styles.row}>
        <Button
          secondary
          title="Sebelumnya"
          disabled={current === 0}
          onPress={() => setIndex(current - 1)}
        />
        <Button
          secondary
          title="Berikutnya"
          disabled={current === session.plan.items.length - 1}
          onPress={() => setIndex(current + 1)}
        />
      </View>
      <Button title="Akhiri sesi" onPress={finishSession} />
      {!!s.error && <Notice error message={s.error} />}
    </Screen>
  );
}
