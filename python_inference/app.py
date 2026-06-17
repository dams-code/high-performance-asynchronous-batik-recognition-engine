from flask import Flask, request, jsonify
import tensorflow as tf
import numpy as np
from PIL import Image
import io

app = Flask(__name__)

MODEL_PATH = "saved_model_batik"
model = tf.saved_model.load(MODEL_PATH)
infer = model.signatures["serving_default"]

CLASS_NAMES = ["Kawung", "Megamendung", "Parang"]

@app.route("/status-python")
def pythonAlive():
    return jsonify({
        "status-service": True,
        "pesan":          "Model CNN Python sudah menyala",
    }), 200

@app.route("/predict", methods=["POST"])
def predict():
    if "image" not in request.files:
        return jsonify({"error": "Gambar tidak ditemukan"}), 400
    
    file = request.files["image"].read()
    # 1. Baca gambar asli dan ubah ke Grayscale (L) murni terlebih dahulu
    # image = Image.open(io.BytesIO(file)).convert("L")
    image = Image.open(io.BytesIO(file)).convert("RGB")
    image = image.resize((150, 150))
    
    # 2. Ubah ke array numpy (0-255) tanpa pembagian 255.0 (karena di modelmu tidak ada Layer Rescaling)
    img_array = np.array(image).astype("float32")
    
    # 3. Tambahkan dimensi batch agar menjadi (1, 150, 150, 3)
    img_array = np.expand_dims(img_array, axis=0)
    
    input_tensor = tf.convert_to_tensor(img_array)
    input_key = list(infer.structured_input_signature[1].keys())[0]
    
    try:
        # Jalankan prediksi ke model .pb
        predictions = infer(**{input_key: input_tensor})
        output_key = list(predictions.keys())[0]
        probabilitas = predictions[output_key].numpy()[0]
        
        best_index = np.argmax(probabilitas)
        hasil_batik = CLASS_NAMES[best_index]
        confidence = float(probabilitas[best_index])
        
        # Cetak log DEBUG murni ke terminal Python
        print(f"\n==============================")
        print(f" Matriks Probabilitas Mentah : {probabilitas}")
        print(f" Indeks Pemenang (0/1/2)     : {best_index}")
        print(f" Nama Batik Hasil Terjemahan : {hasil_batik}")
        print(f"==============================\n")
        
        return jsonify({
            "batik_type": hasil_batik,
            "confidence": confidence
        })
    except Exception as e:
        print(f"[ERROR] Gagal inferensi: {str(e)}")
        return jsonify({"error": str(e)}), 500

if __name__ == "__main__":
    app.run(port=5005, debug=True)
