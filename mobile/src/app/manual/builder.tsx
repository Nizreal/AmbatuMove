import { useState } from "react";
import { View } from "react-native";
import { Alert } from "../../components/confirm";
import { router } from "expo-router";
import {
  Button,
  Input,
  Notice,
  Screen,
  Txt,
  styles,
} from "../../components/ui";
import { ExercisePhoto } from "../../components/exercise";
import { useStore } from "../../hooks/store";
import {
  PlanItem,
  begin,
  estimate,
  uid,
  validateItem,
} from "../../domain/workout";
import { colors } from "../../theme";

function Parameters({
  item,
  onChange,
}: {
  item: PlanItem;
  onChange: (p: PlanItem) => void;
}) {
  return (
    <View style={styles.group}>
      <View style={styles.manualFields}>
        <Input
          label="Set (1–10)"
          keyboardType="number-pad"
          value={item.sets ? String(item.sets) : ""}
          onChangeText={(t) => onChange({ ...item, sets: Number(t) })}
        />
        <Input
          label={item.exercise.timed ? "Durasi (detik)" : "Repetisi"}
          keyboardType="number-pad"
          value={item.target ? String(item.target) : ""}
          onChangeText={(t) => onChange({ ...item, target: Number(t) })}
        />
        <Input
          label="Jeda (detik)"
          keyboardType="number-pad"
          value={String(item.rest)}
          onChangeText={(t) => onChange({ ...item, rest: Number(t) })}
        />
      </View>
      {!validateItem(item) && (
        <Notice
          error
          message="Gunakan bilangan bulat dalam rentang yang tertera."
        />
      )}
    </View>
  );
}

export default function Builder() {
  const s = useStore();
  const [error, setError] = useState("");
  function createDraft() {
    s.setPlan({
      id: uid(),
      title: "Latihan pilihan saya",
      source: "manual",
      items: [],
      createdAt: new Date().toISOString(),
    });
  }
  const p = s.plan;
  if (!p || p.source !== "manual")
    return (
      <Screen>
        <Txt variant="heading">Mulai latihan pilihan Anda</Txt>
        <Button title="Buat draf manual" onPress={createDraft} />
      </Screen>
    );
  const plan = p;
  const update = (items: PlanItem[]) => s.setPlan({ ...plan, items });
  function move(i: number, d: number) {
    const items = [...plan.items];
    [items[i], items[i + d]] = [items[i + d], items[i]];
    update(items);
  }
  function start() {
    setError("");
    if (s.active) {
      router.push("/workout/active");
      return;
    }
    if (
      !plan.title.trim() ||
      !plan.items.length ||
      !plan.items.every(validateItem)
    ) {
      setError("Isi nama dan minimal satu gerakan dengan target yang valid.");
      return;
    }
    s.setActive(begin(plan));
    router.push("/workout/active");
  }
  return (
    <Screen
      footer={
        <Button
          title={s.active ? "Lanjutkan sesi aktif" : "Mulai workout manual"}
          onPress={start}
        />
      }
    >
      <Txt variant="title">Workout manual</Txt>
      <Input
        label="Nama latihan"
        maxLength={60}
        value={plan.title}
        onChangeText={(title) => s.setPlan({ ...plan, title })}
      />
      <Txt color={colors.muted}>
        {plan.items.length} gerakan · sekitar {estimate(plan.items)} menit
      </Txt>
      {!plan.items.length && (
        <Notice message="Draf masih kosong. Tambahkan gerakan dari katalog untuk menyusun sesi." />
      )}
      {plan.items.map((item, index) => (
        <View style={styles.manualCard} key={item.exercise.id}>
          <Txt variant="small" color={colors.primary}>
            Gerakan {index + 1}
          </Txt>
          <View style={styles.planRow}>
            <ExercisePhoto exercise={item.exercise} compact />
            <View style={{ flex: 1, gap: 3 }}>
              <Txt variant="exercise">{item.exercise.name}</Txt>
              <Txt variant="small" color={colors.muted}>
                {item.exercise.muscle} · {item.exercise.level}
              </Txt>
              <Txt variant="small">
                {item.sets} set × {item.target}{" "}
                {item.exercise.timed ? "detik" : "repetisi"}
              </Txt>
            </View>
          </View>
          <Parameters
            item={item}
            onChange={(next) =>
              update(plan.items.map((v, i) => (i === index ? next : v)))
            }
          />
          <View style={styles.row}>
            <Button
              secondary
              title="Panduan"
              onPress={() =>
                router.push({
                  pathname: "/exercise/[id]",
                  params: { id: item.exercise.id },
                })
              }
            />
            <Button
              secondary
              title="Naik"
              disabled={index === 0}
              onPress={() => move(index, -1)}
            />
            <Button
              secondary
              title="Turun"
              disabled={index === plan.items.length - 1}
              onPress={() => move(index, 1)}
            />
            <Button
              secondary
              title="Hapus"
              onPress={() =>
                Alert.alert("Hapus gerakan?", item.exercise.name, [
                  { text: "Batal", style: "cancel" },
                  {
                    text: "Hapus",
                    style: "destructive",
                    onPress: () =>
                      update(plan.items.filter((_, i) => i !== index)),
                  },
                ])
              }
            />
          </View>
        </View>
      ))}
      <Button
        secondary
        title="Tambah dari katalog"
        onPress={() => router.push("/catalog")}
      />
      {!!error && <Notice error message={error} />}
    </Screen>
  );
}
