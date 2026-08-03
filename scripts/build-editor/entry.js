import { EditorView, basicSetup } from "codemirror";
import { EditorState, Compartment } from "@codemirror/state";
import { go } from "@codemirror/lang-go";
import { lintGutter, setDiagnostics } from "@codemirror/lint";

window.CM = { EditorView, EditorState, Compartment, basicSetup, go, lintGutter, setDiagnostics };
