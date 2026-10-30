"use client";

import { MeshGradient } from "@paper-design/shaders-react";

import { ShaderStage } from "@/components/brand/shader-stage";
import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

/**
 * Animated "signal on ink" mesh behind the guest pages: ink, deep greens and
 * a thin vein of the signal green (docs/brand.md). Mounted lazily, paused
 * off-screen and still under reduced motion; the ink background shows until
 * WebGL draws.
 */
export function AuthBackdrop({ className }: { className?: string }) {
  return (
    <ShaderStage className={cn("bg-ink", className)}>
      {(active) => (
        <MeshGradient
          className="size-full"
          colors={[...brand.shader.mesh]}
          distortion={0.8}
          swirl={0.45}
          grainMixer={0.18}
          grainOverlay={0.08}
          speed={active ? 0.28 : 0}
          maxPixelCount={1920 * 1080}
        />
      )}
    </ShaderStage>
  );
}
