import { createApp } from 'vue'
import App from './App.vue'
import router from "./router/index.js";
import ElementPlus, {ElMessage} from 'element-plus'
import 'element-plus/dist/index.css'
import 'dayjs/locale/zh-cn'
import zhCn from 'element-plus/dist/locale/zh-cn.mjs'
import axios from 'axios'
import VueMarkdownEditor from '@kangc/v-md-editor';
import '@kangc/v-md-editor/lib/style/base-editor.css';
import vuepressTheme from '@kangc/v-md-editor/lib/theme/vuepress.js';
import '@kangc/v-md-editor/lib/theme/style/vuepress.css';
import Prism from 'prismjs';
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import "./assets/main.css"
import '@fortawesome/fontawesome-free/css/all.min.css';
VueMarkdownEditor.use(vuepressTheme, {
    Prism,
});
const app=createApp(App);
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
    app.component(key, component)
}
app.use(ElementPlus, {
    locale: zhCn,
})
app.use(VueMarkdownEditor);
axios.defaults.baseURL = '/back';
axios.loadData = async function (url) {
    const resp = await axios.get(url);
    return resp.data;
};
axios.interceptors.request.use(function (config) {

    if(localStorage.getItem("token"))
        config.headers.token =  localStorage.getItem("token");
    return config;
}, function (error) {
    return Promise.reject(error);
});
// 响应拦截器
axios.interceptors.response.use(
    function (response) {

        if(response.data.code==401){
            ElMessage.error( "登陆异常")
            router.push("/login")
            response.status.code=401
            return response;
        }

        if( response.data.success== false){
            ElMessage.error( response.data.message)
        }
        // 对响应数据做点什么
        return response;
    },
    function (error) {

        return Promise.reject(error);
    }
);

app.config.globalProperties.$http = axios;
app.use(router);
app.use(ElementPlus);
app.mount("#app");
