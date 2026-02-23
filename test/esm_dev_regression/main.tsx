// Core React
import React from "react";
import { createRoot } from "react-dom/client";

// Previously broken packages (regression)
import lodashMemoize from "lodash-es/memoize.js";
import { useSyncExternalStoreWithSelector } from "use-sync-external-store/shim/with-selector";
import { EventEmitter } from "events";

// Highlight.js / lowlight
import { common } from "lowlight";

// Simple ESM
import { clsx } from "clsx";
import { format } from "date-fns";
import { v4 as uuidv4 } from "uuid";

// Routing
import { createBrowserRouter } from "react-router-dom";

// React Query
import { QueryClient } from "@tanstack/react-query";

// Tiptap ecosystem
import { useEditor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import CodeBlockLowlight from "@tiptap/extension-code-block-lowlight";
import CharacterCount from "@tiptap/extension-character-count";
import Highlight from "@tiptap/extension-highlight";
import Image from "@tiptap/extension-image";
import Link from "@tiptap/extension-link";
import Placeholder from "@tiptap/extension-placeholder";
import Subscript from "@tiptap/extension-subscript";
import Superscript from "@tiptap/extension-superscript";
import Table from "@tiptap/extension-table";
import TableCell from "@tiptap/extension-table-cell";
import TableHeader from "@tiptap/extension-table-header";
import TableRow from "@tiptap/extension-table-row";
import TextAlign from "@tiptap/extension-text-align";
import Typography from "@tiptap/extension-typography";
import Underline from "@tiptap/extension-underline";
import { Plugin } from "@tiptap/pm/state";
import { Suggestion } from "@tiptap/suggestion";

// Vercel analytics
import { inject } from "@vercel/analytics";
import { injectSpeedInsights } from "@vercel/speed-insights";

// Solana
import { TOKEN_PROGRAM_ID } from "@solana/spl-token";
import { BaseWalletAdapter } from "@solana/wallet-adapter-base";
import { useWallet } from "@solana/wallet-adapter-react";
import { Connection } from "@solana/web3.js";

// Chart.js
import { Chart } from "chart.js";
import { Line } from "react-chartjs-2";

// Web3
import { createConfig } from "wagmi";
import { createPublicClient } from "viem";
import { ConnectKitProvider } from "connectkit";

// Icons
import { Search } from "lucide-react";
import { FaGithub } from "react-icons/fa";

// Markdown
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import rehypeKatex from "rehype-katex";

// Other
import { QRCodeSVG } from "qrcode.react";
import katex from "katex";
import { marked } from "marked";
import { Markdown as TiptapMarkdown } from "tiptap-markdown";

// Animation
import gsap from "gsap";

// API / validation
import createClient from "openapi-fetch";
import { z } from "zod";

// Crypto/encoding
import bs58 from "bs58";
import { Buffer } from "buffer";

// Heavy packages
import mermaid from "mermaid";

// Emotion (CSS-in-JS)
import { css } from "@emotion/react";
import styled from "@emotion/styled";

// Framer Motion
import { motion } from "framer-motion";

// Zustand
import { create } from "zustand";

// Recharts
import { LineChart, BarChart } from "recharts";

// React Hook Form
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";

// shadcn/UI deps
import { cva } from "class-variance-authority";
import { twMerge } from "tailwind-merge";

// Radix UI primitives
import { Dialog } from "@radix-ui/react-dialog";
import { Popover } from "@radix-ui/react-popover";
import { Slot } from "@radix-ui/react-slot";
import { Tooltip } from "@radix-ui/react-tooltip";
import { DropdownMenu } from "@radix-ui/react-dropdown-menu";
import { Select } from "@radix-ui/react-select";
import { Tabs } from "@radix-ui/react-tabs";
import { Accordion } from "@radix-ui/react-accordion";

// Command palette / toast / carousel / etc
import { Command } from "cmdk";
import { toast } from "sonner";
import useEmblaCarousel from "embla-carousel-react";
import { OTPInput } from "input-otp";
import { DayPicker } from "react-day-picker";
import { Drawer } from "vaul";
import { createColumnHelper } from "@tanstack/react-table";

// Force all imports to be retained by esbuild — reference every imported value.
const imports = {
  React, createRoot,
  lodashMemoize, useSyncExternalStoreWithSelector, EventEmitter,
  common,
  clsx, format, uuidv4,
  createBrowserRouter,
  QueryClient,
  useEditor, StarterKit, CodeBlockLowlight, CharacterCount, Highlight,
  Image, Link, Placeholder, Subscript, Superscript,
  Table, TableCell, TableHeader, TableRow, TextAlign, Typography, Underline,
  Plugin, Suggestion,
  inject, injectSpeedInsights,
  TOKEN_PROGRAM_ID, BaseWalletAdapter, useWallet, Connection,
  Chart, Line,
  createConfig, createPublicClient, ConnectKitProvider,
  Search, FaGithub,
  ReactMarkdown, remarkGfm, remarkMath, rehypeKatex,
  QRCodeSVG, katex, marked, TiptapMarkdown,
  gsap,
  createClient, z,
  bs58, Buffer,
  mermaid,
  css, styled,
  motion,
  create,
  LineChart, BarChart,
  useForm, zodResolver,
  cva, twMerge,
  Dialog, Popover, Slot, Tooltip, DropdownMenu, Select, Tabs, Accordion,
  Command, toast, useEmblaCarousel, OTPInput, DayPicker, Drawer,
  createColumnHelper,
};

console.log(`ESM regression: ${Object.keys(imports).length} imports loaded`);
