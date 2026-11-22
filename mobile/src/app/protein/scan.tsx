import { CameraView, useCameraPermissions } from "expo-camera";
import { router } from "expo-router";
import { useState } from "react";
import { TextInput, View } from "react-native";
import { Button, Notice, Screen, Txt, styles } from "../../components/ui";
import { colors, space } from "../../theme";

export default function ProteinBarcodeScanner() {
  const [permission, requestPermission] = useCameraPermissions();
  const [scanned, setScanned] = useState("");
  const [manualCode, setManualCode] = useState("");
  if (!permission)
    return (
      <Screen>
        <Txt>Memeriksa izin kamera…</Txt>
      </Screen>
    );
  if (!permission.granted)
    return (
      <Screen>
        <Txt variant="title">Pindai barcode</Txt>
        <Notice message="Kamera hanya digunakan untuk membaca barcode produk dan tidak mengambil foto." />
        {permission.canAskAgain ? (
          <Button
            title="Izinkan kamera"
            onPress={() => void requestPermission()}
          />
        ) : (
          <Notice
            error
            message="Izin kamera dimatikan. Aktifkan izin kamera di pengaturan perangkat."
          />
        )}
        <TextInput
          accessibilityLabel="Barcode produk"
          keyboardType="number-pad"
          value={manualCode}
          onChangeText={setManualCode}
          placeholder="Masukkan 8–14 angka"
          placeholderTextColor={colors.muted}
          style={styles.input}
        />
        <Button
          secondary
          title="Gunakan barcode"
          onPress={() => {
            const normalized = manualCode.replace(/\s/g, "");
            if (/^[0-9]{8,14}$/.test(normalized))
              router.navigate({
                pathname: "/protein",
                params: { barcode: normalized },
              });
          }}
        />
      </Screen>
    );
  const applyCode = (value: string) => {
    const normalized = value.replace(/\s/g, "");
    if (!/^[0-9]{8,14}$/.test(normalized)) return;
    router.navigate({ pathname: "/protein", params: { barcode: normalized } });
  };
  return (
    <Screen>
      <Txt variant="title">Pindai barcode</Txt>
      <Notice message="Arahkan barcode produk ke kamera. Barcode hanya mengisi kode; nilai protein tetap harus dibaca dari kemasan sampai lookup produk tersedia." />
      {!scanned && (
        <View
          style={{
            height: 360,
            overflow: "hidden",
            borderRadius: 18,
            backgroundColor: colors.text,
          }}
        >
          <CameraView
            style={{ flex: 1 }}
            facing="back"
            barcodeScannerSettings={{
              barcodeTypes: ["ean13", "ean8", "upc_a", "upc_e"],
            }}
            onBarcodeScanned={({ data }) => setScanned(data)}
          />
        </View>
      )}
      {scanned && scanned !== "manual" && (
        <View style={styles.planCard}>
          <Txt variant="label">Kode terbaca</Txt>
          <Txt variant="heading">{scanned}</Txt>
          <Button title="Gunakan kode" onPress={() => applyCode(scanned)} />
          <Button
            secondary
            title="Pindai lagi"
            onPress={() => setScanned("")}
          />
        </View>
      )}
      {(scanned === "manual" || !scanned) && (
        <View style={{ gap: space.sm }}>
          {scanned === "manual" && (
            <Txt variant="label">Masukkan angka barcode</Txt>
          )}
          {scanned === "manual" && (
            <TextInput
              accessibilityLabel="Barcode produk"
              keyboardType="number-pad"
              value={manualCode}
              onChangeText={setManualCode}
              placeholder="8–14 angka"
              placeholderTextColor={colors.muted}
              style={styles.input}
            />
          )}
          {scanned === "manual" && (
            <Button
              title="Gunakan barcode"
              onPress={() => applyCode(manualCode)}
            />
          )}
          {!scanned && (
            <Button
              secondary
              title="Masukkan barcode manual"
              onPress={() => setScanned("manual")}
            />
          )}
        </View>
      )}
      <Button secondary title="Kembali" onPress={() => router.back()} />
    </Screen>
  );
}
