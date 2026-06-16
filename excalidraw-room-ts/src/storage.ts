import {
  CreateBucketCommand,
  GetObjectCommand,
  HeadBucketCommand,
  PutObjectCommand,
  S3Client,
} from "@aws-sdk/client-s3";

const BUCKET = process.env.STORAGE_BUCKET ?? "excalidraw-rooms";
const IV_BYTES = 12; // AES-GCM IV is always 12 bytes

const s3 = new S3Client({
  endpoint: process.env.STORAGE_ENDPOINT ?? "http://rustfs:9000",
  region: "us-east-1",
  credentials: {
    accessKeyId: process.env.STORAGE_ACCESS_KEY ?? "rustfsadmin",
    secretAccessKey: process.env.STORAGE_SECRET_KEY ?? "rustfsadmin",
  },
  forcePathStyle: true,
});

export async function initStorage(retries = 10, delayMs = 3000): Promise<void> {
  for (let i = 0; i < retries; i++) {
    try {
      try {
        await s3.send(new HeadBucketCommand({ Bucket: BUCKET }));
      } catch {
        await s3.send(new CreateBucketCommand({ Bucket: BUCKET }));
        console.log(`storage: created bucket "${BUCKET}"`);
      }
      console.log(`storage: ready (bucket "${BUCKET}")`);
      return;
    } catch {
      const wait = delayMs * (i + 1);
      console.log(`storage: not ready, retry ${i + 1}/${retries} in ${wait}ms...`);
      await new Promise((r) => setTimeout(r, wait));
    }
  }
  console.error("storage: failed to initialize — running without persistence");
}

// roomID → pending save timer
const saveTimers = new Map<string, ReturnType<typeof setTimeout>>();

export function scheduleSave(
  roomID: string,
  encryptedData: ArrayBuffer,
  iv: Uint8Array,
  debounceMs = 1000,
): void {
  const existing = saveTimers.get(roomID);
  if (existing) clearTimeout(existing);

  saveTimers.set(
    roomID,
    setTimeout(() => {
      saveTimers.delete(roomID);
      save(roomID, encryptedData, iv).catch((err) =>
        console.error(`storage: save failed for room ${roomID}:`, err),
      );
    }, debounceMs),
  );
}

async function save(roomID: string, encryptedData: ArrayBuffer, iv: Uint8Array): Promise<void> {
  // Layout: [IV (12 bytes)][encryptedData]
  const body = Buffer.concat([Buffer.from(iv), Buffer.from(encryptedData)]);
  await s3.send(
    new PutObjectCommand({
      Bucket: BUCKET,
      Key: `rooms/${roomID}/state`,
      Body: body,
      ContentType: "application/octet-stream",
    }),
  );
}

export async function loadRoomState(
  roomID: string,
): Promise<{ encryptedData: ArrayBuffer; iv: Uint8Array } | null> {
  try {
    const resp = await s3.send(
      new GetObjectCommand({ Bucket: BUCKET, Key: `rooms/${roomID}/state` }),
    );
    if (!resp.Body) return null;

    const bytes = await (
      resp.Body as NodeJS.ReadableStream & { transformToByteArray(): Promise<Uint8Array> }
    ).transformToByteArray();
    const iv = bytes.slice(0, IV_BYTES);
    const encryptedData = bytes.slice(IV_BYTES);
    return { iv: new Uint8Array(iv), encryptedData: encryptedData.buffer as ArrayBuffer };
  } catch {
    return null;
  }
}
