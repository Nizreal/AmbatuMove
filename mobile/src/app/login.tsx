import { useState } from "react";
import { Image, Pressable, View } from "react-native";
import { Button, Input, Notice, Screen, Txt, styles } from "../components/ui";
import { DEMO, mockAuth } from "../services/mock";
import { useStore } from "../hooks/store";
import { colors, space } from "../theme";

export default function Login() {
  const s = useStore();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [show, setShow] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit() {
    setError("");
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()))
      return setError("Masukkan alamat email yang valid.");
    if (!password) return setError("Masukkan kata sandi demo.");
    setBusy(true);
    try {
      await s.login(await mockAuth.login(email, password));
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "Tidak dapat masuk. Coba lagi.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <Screen standalone>
      <View style={styles.loginBrand}>
        <Image
          source={require("../../assets/brand.png")}
          resizeMode="contain"
          style={{ width: 56, height: 56 }}
          accessibilityLabel="AmbatuMove"
        />
        <Txt variant="heading" color={colors.primary}>
          AmbatuMove
        </Txt>
      </View>
      <View style={{ gap: space.sm }}>
        <Txt variant="title">
          Selamat datang di{" "}
          <Txt variant="title" color={colors.primary}>
            AmbatuMove
          </Txt>
        </Txt>
        <Txt color={colors.muted}>
          Rencanakan latihan, jalani setiap gerakan, dan pantau progres Anda.
        </Txt>
      </View>
      <View style={styles.group}>
        <Input
          label="Email"
          testID="email"
          value={email}
          onChangeText={setEmail}
          autoCapitalize="none"
          keyboardType="email-address"
          autoComplete="email"
          returnKeyType="next"
        />
        <View style={{ position: "relative" }}>
          <Input
            label="Kata sandi"
            testID="password"
            value={password}
            onChangeText={setPassword}
            secureTextEntry={!show}
            autoCapitalize="none"
            returnKeyType="done"
            onSubmitEditing={() => void submit()}
            style={{ paddingRight: 54 }}
          />
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={
              show ? "Sembunyikan kata sandi" : "Tampilkan kata sandi"
            }
            onPress={() => setShow(!show)}
            style={styles.passwordToggle}
          >
            <Txt variant="small" color={colors.primary}>
              {show ? "Sembunyikan" : "Lihat"}
            </Txt>
          </Pressable>
        </View>
      </View>
      {!!error && <Notice error message={error} />}
      <Button
        title="Masuk"
        testID="login"
        busy={busy}
        onPress={() => void submit()}
      />
      <Pressable
        accessibilityRole="button"
        onPress={() => {
          setEmail(DEMO.email);
          setPassword(DEMO.password);
        }}
        style={styles.demoBox}
      >
        <View style={{ flex: 1, gap: 2 }}>
          <Txt variant="label" color={colors.primary}>
            Isi akun demo
          </Txt>
          <Txt variant="small" color={colors.muted}>
            {DEMO.email} · {DEMO.password}
          </Txt>
        </View>
      </Pressable>
    </Screen>
  );
}
