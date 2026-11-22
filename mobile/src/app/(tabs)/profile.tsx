import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import { router } from "expo-router";
import { Pressable, View } from "react-native";
import { Alert } from "../../components/confirm";
import { Avatar, Screen, Txt, styles } from "../../components/ui";
import { useStore } from "../../hooks/store";
import { colors, space } from "../../theme";

function initials(email: string) {
  const parts = email.split("@")[0].split(/[._-]/).filter(Boolean);
  return (
    parts.length > 1 ? parts[0][0] + parts[1][0] : parts[0]?.slice(0, 2) || "AM"
  ).toUpperCase();
}

export default function Profile() {
  const s = useStore();
  return (
    <Screen
      footer={
        <Pressable
          accessibilityRole="button"
          onPress={() =>
            Alert.alert(
              "Keluar dari AmbatuMove?",
              "Draf dan riwayat latihan tetap tersimpan di perangkat.",
              [
                { text: "Batal", style: "cancel" },
                {
                  text: "Keluar",
                  style: "destructive",
                  onPress: () =>
                    void s
                      .logout()
                      .then(() => router.replace("/login"))
                      .catch(() =>
                        s.setError("Tidak dapat keluar. Coba lagi."),
                      ),
                },
              ],
            )
          }
          style={({ pressed }) => [
            styles.logoutButton,
            pressed && { backgroundColor: colors.primarySoft },
          ]}
        >
          <MaterialCommunityIcons
            name="logout"
            size={20}
            color={colors.primary}
          />
          <Txt variant="label" color={colors.primary}>
            Keluar akun
          </Txt>
        </Pressable>
      }
    >
      <Txt variant="title">Profil</Txt>
      <View style={styles.profileIntro}>
        <Avatar label={initials(s.user ?? "AmbatuMove")} size={104} />
        <Txt variant="heading">Akun Anda</Txt>
        <Txt color={colors.muted}>{s.user ?? ""}</Txt>
      </View>
      <View style={styles.profileEmail}>
        <MaterialCommunityIcons
          name="email-outline"
          size={22}
          color={colors.primary}
        />
        <View style={{ flex: 1, gap: 2 }}>
          <Txt variant="small" color={colors.muted}>
            Email akun
          </Txt>
          <Txt variant="label">{s.user ?? "—"}</Txt>
        </View>
      </View>
      {!!s.error && <Txt color={colors.error}>{s.error}</Txt>}
      <Txt
        variant="small"
        color={colors.muted}
        style={{ textAlign: "center", marginTop: space.md }}
      >
        Akun demo tersimpan lokal.
      </Txt>
    </Screen>
  );
}
