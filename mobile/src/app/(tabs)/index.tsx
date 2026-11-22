import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { Pressable, View } from "react-native";
import {
  Avatar,
  Button,
  Notice,
  Screen,
  Txt,
  styles,
} from "../../components/ui";
import { useStore } from "../../hooks/store";
import { colors, space } from "../../theme";
import { uid } from "../../domain/workout";

function Action({
  icon,
  title,
  detail,
  onPress,
}: {
  icon: keyof typeof MaterialCommunityIcons.glyphMap;
  title: string;
  detail: string;
  onPress: () => void;
}) {
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      style={({ pressed }) => [
        styles.dashboardAction,
        pressed && { backgroundColor: colors.primarySoft },
      ]}
    >
      <View style={styles.dashboardActionIcon}>
        <MaterialCommunityIcons name={icon} size={21} color={colors.primary} />
      </View>
      <View style={{ flex: 1, gap: 2 }}>
        <Txt variant="label">{title}</Txt>
        <Txt variant="small" color={colors.muted}>
          {detail}
        </Txt>
      </View>
      <MaterialCommunityIcons
        name="chevron-right"
        size={23}
        color={colors.muted}
      />
    </Pressable>
  );
}

export default function Home() {
  const s = useStore();
  const avatarInitial = (s.user?.split("@")[0]?.charAt(0) || "A").toUpperCase();
  return (
    <Screen>
      <View style={styles.dashboardHeader}>
        <View style={{ gap: 2 }}>
          <Txt variant="small" color={colors.muted}>
            Halo,
          </Txt>
          <Txt variant="heading">Selamat datang!</Txt>
        </View>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel="Buka profil"
          onPress={() => router.push("/(tabs)/profile")}
        >
          <Avatar label={avatarInitial} />
        </Pressable>
      </View>
      {!!s.error && <Notice error message={s.error} />}
      {!!s.error && (
        <Button
          secondary
          title="Coba simpan lagi"
          onPress={() => void s.retrySave()}
        />
      )}
      <View style={styles.hero}>
        <View style={{ gap: space.sm }}>
          <Txt variant="heading">Siap bergerak hari ini?</Txt>
          <Txt color={colors.text}>
            Pilih latihan sesuai waktu dan alat yang Anda punya.
          </Txt>
        </View>
      </View>
      {s.active ? (
        <Button
          title="Lanjutkan sesi aktif"
          onPress={() => router.push("/workout/active")}
        />
      ) : null}
      <View style={{ gap: space.sm }}>
        <Action
          icon="dumbbell"
          title="Rekomendasi workout"
          detail="Rencana sesuai kondisi Anda"
          onPress={() => router.push("/recommendation/form")}
        />
        <Action
          icon="format-list-bulleted"
          title="Workout manual"
          detail="Susun gerakan pilihan sendiri"
          onPress={() => {
            if (s.plan?.source !== "manual")
              s.setPlan({
                id: uid(),
                title: "Latihan pilihan saya",
                source: "manual",
                items: [],
                createdAt: new Date().toISOString(),
              });
            router.push("/manual/builder");
          }}
        />
      </View>
    </Screen>
  );
}
