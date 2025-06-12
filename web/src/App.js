import React, { useEffect, useRef } from "react";
import JSMpeg from "jsmpeg";

function App() {
  const canvasRef = useRef(null);

  useEffect(() => {
    const player = new JSMpeg.Player("ws://" + window.location.host + "/ws", {
      canvas: canvasRef.current,
      autoplay: true,
      audio: false,
    });
    return () => {
      player.destroy();
    };
  }, []);

  return (
    <div style={{ background: "#000", height: "100vh", margin: 0 }}>
      <canvas
        ref={canvasRef}
        id="video-canvas"
        style={{
          display: "block",
          margin: "0 auto",
          background: "#000",
          width: "100vw",
          height: "100vh",
        }}
      />
    </div>
  );
}

export default App;
