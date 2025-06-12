import React, { useEffect, useRef, useState } from "react";
import JSMpeg from 'jsmpeg';

function App() {
  const canvasRef = useRef(null);
  const [status, setStatus] = useState("Connecting...");

  useEffect(() => {
    let player = null;

    const initializePlayer = () => {
      try {
        const url = `ws://${window.location.host}/ws`;
        console.log("Connecting to:", url);
        setStatus(`Connecting to ${url}`);

        // Use JSMpeg directly as imported (it's the Player constructor)
        player = new JSMpeg.Player(url, {
          canvas: canvasRef.current,
          autoplay: true,
          audio: false,
          onVideoDecode: function(decoder, time) {
            setStatus(`Live - ${decoder.width}x${decoder.height}`);
          },
          onSourceEstablished: function() {
            setStatus("Connected, waiting for video...");
          },
          onSourceError: function() {
            setStatus("WebSocket connection failed");
          }
        });
      } catch (error) {
        console.error("Failed to initialize player:", error);
        setStatus("Failed to initialize video player");
      }
    };

    initializePlayer();

    return () => {
      if (player && typeof player.destroy === "function") {
        player.destroy();
      }
    };
  }, []);

  return (
    <div style={{ 
      background: "#000", 
      height: "100vh", 
      margin: 0,
      display: "flex",
      flexDirection: "column",
      alignItems: "center",
      justifyContent: "center"
    }}>
      <div style={{ 
        color: "white", 
        position: "absolute", 
        top: "10px", 
        left: "10px",
        zIndex: 1000,
        fontSize: "14px",
        fontFamily: "monospace"
      }}>
        {status}
      </div>
      <canvas
        ref={canvasRef}
        style={{
          maxWidth: "100%",
          maxHeight: "100%",
          background: "#000",
          border: "1px solid #333"
        }}
      />
    </div>
  );
}

export default App;
